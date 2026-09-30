// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"testing"
)

func TestRFCAcceptValue(t *testing.T) {
	if got := acceptNonce("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept = %s", got)
	}
}

func TestMaskRoundTripCoversEveryByte(t *testing.T) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}
	key := []byte{1, 2, 3, 4}
	masked := applyMask(payload, key)
	if bytes.Equal(masked, payload) {
		t.Fatal("mask did not change the payload")
	}
	if !bytes.Equal(applyMask(masked, key), payload) {
		t.Fatal("mask is not its own inverse")
	}
}

func TestByteAtATimeBinaryFrame(t *testing.T) {
	payload := make([]byte, 256+64*1024)
	for i := 0; i < 256; i++ {
		payload[i] = byte(i)
	}
	if _, err := rand.Read(payload[256:]); err != nil {
		t.Fatal(err)
	}
	encoded, err := clientFrame(opBinary, payload)
	if err != nil {
		t.Fatal(err)
	}
	parser := &frameParser{}
	var frames []frame
	for i := range encoded {
		got, err := parser.feed(encoded[i : i+1])
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, got...)
	}
	if len(frames) != 1 || !frames[0].fin || frames[0].opcode != opBinary || !bytes.Equal(frames[0].payload, payload) {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestUnmaskedEmptyFrame(t *testing.T) {
	encoded, err := encodeFrame(opBinary, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := (&frameParser{}).feed(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || !frames[0].fin || frames[0].opcode != opBinary || len(frames[0].payload) != 0 {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestPingBetweenFragmentsStaysOutOfTheByteStream(t *testing.T) {
	seq := bytes.Repeat([]byte{0}, 256)
	for i := range seq {
		seq[i] = byte(i)
	}
	chunks := []struct {
		op  int
		fin bool
		p   []byte
	}{
		{opBinary, false, []byte("AAA")},
		{opPing, true, []byte("xyz")},
		{opPong, true, []byte("xyz")},
		{opContinuation, false, seq},
		{opPing, true, []byte{0, 0xff}},
		{opContinuation, true, []byte("BBB")},
	}
	var wire []byte
	for _, chunk := range chunks {
		encoded, err := encodeFrame(chunk.op, chunk.p, chunk.fin, nil)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, encoded...)
	}
	parser := &frameParser{}
	var frames []frame
	for i := range wire {
		got, err := parser.feed(wire[i : i+1])
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, got...)
	}
	stream := &byteStream{}
	var out []byte
	var controls []frame
	for _, fr := range frames {
		if fr.opcode == opPing || fr.opcode == opPong || fr.opcode == opClose {
			controls = append(controls, fr)
			continue
		}
		piece, err := stream.push(fr.fin, fr.opcode, fr.payload)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, piece...)
	}
	want := append(append([]byte("AAA"), seq...), []byte("BBB")...)
	if !bytes.Equal(out, want) {
		t.Fatalf("stream len %d", len(out))
	}
	if len(controls) != 3 || !bytes.Equal(controls[0].payload, []byte("xyz")) || controls[0].opcode != opPing {
		t.Fatalf("controls = %+v", controls)
	}
}

func TestTextAndReservedBitsAreRejected(t *testing.T) {
	if _, err := (&byteStream{}).push(true, opText, []byte("nope")); err == nil {
		t.Fatal("text frame was accepted")
	}
	dirty, err := encodeFrame(opBinary, []byte("abc"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty[0] |= 0x40
	if _, err := (&frameParser{}).feed(dirty); err == nil {
		t.Fatal("reserved bits were accepted")
	}
}

func TestSixteenAndSixtyFourBitLengths(t *testing.T) {
	medium := make([]byte, 200)
	if _, err := rand.Read(medium); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeFrame(opBinary, medium, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[1]&0x7F != 126 || binary.BigEndian.Uint16(encoded[2:4]) != 200 {
		t.Fatalf("medium header = %v", encoded[:4])
	}
	frames, err := (&frameParser{}).feed(encoded)
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0].payload, medium) {
		t.Fatalf("medium parse failed: %v", err)
	}
	payload := []byte("xyz")
	header := []byte{0x82, 127}
	header = append(header, make([]byte, 8)...)
	binary.BigEndian.PutUint64(header[2:], uint64(len(payload)))
	frames, err = (&frameParser{}).feed(append(header, payload...))
	if err != nil || len(frames) != 1 || !bytes.Equal(frames[0].payload, payload) {
		t.Fatalf("wide parse failed: %v", err)
	}
}

func TestContinuationWithoutAStartIsRejected(t *testing.T) {
	if _, err := (&byteStream{}).push(true, opContinuation, []byte("x")); err == nil {
		t.Fatal("orphan continuation was accepted")
	}
}
