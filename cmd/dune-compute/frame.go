// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

const (
	opContinuation = 0
	opText         = 1
	opBinary       = 2
	opClose        = 8
	opPing         = 9
	opPong         = 10
	maxPayload     = 8 * 1024 * 1024
	websocketGUID  = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

type protocolError string

func (e protocolError) Error() string { return string(e) }

func acceptNonce(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func applyMask(payload, key []byte) []byte {
	out := make([]byte, len(payload))
	for i, b := range payload {
		out[i] = b ^ key[i&3]
	}
	return out
}

type frame struct {
	fin     bool
	opcode  int
	payload []byte
}

func encodeFrame(opcode int, payload []byte, fin bool, mask []byte) ([]byte, error) {
	if opcode < 0 || opcode > 15 {
		return nil, protocolError("opcode")
	}
	if len(payload) > maxPayload {
		return nil, protocolError("frame too large")
	}
	control := opcode == opClose || opcode == opPing || opcode == opPong
	if control && (!fin || len(payload) > 125) {
		return nil, protocolError("bad control frame")
	}
	first := byte(opcode)
	if fin {
		first |= 0x80
	}
	header := []byte{first, 0}
	length := len(payload)
	if mask != nil {
		header[1] |= 0x80
	}
	switch {
	case length < 126:
		header[1] |= byte(length)
	case length < 65536:
		header[1] |= 126
		header = append(header, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(length))
	default:
		header[1] |= 127
		header = append(header, make([]byte, 8)...)
		binary.BigEndian.PutUint64(header[len(header)-8:], uint64(length))
	}
	if mask == nil {
		return append(header, payload...), nil
	}
	if len(mask) != 4 {
		return nil, protocolError("mask key")
	}
	return append(append(header, mask...), applyMask(payload, mask)...), nil
}

func clientFrame(opcode int, payload []byte) ([]byte, error) {
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return nil, err
	}
	return encodeFrame(opcode, payload, true, mask)
}

func closePayload(code int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(code))
	return buf
}

type frameParser struct {
	buf []byte
}

func (p *frameParser) feed(data []byte) ([]frame, error) {
	if len(data) > 0 {
		p.buf = append(p.buf, data...)
	}
	var frames []frame
	for {
		fr, ok, err := p.pop()
		if err != nil || !ok {
			return frames, err
		}
		frames = append(frames, fr)
	}
}

func (p *frameParser) pop() (frame, bool, error) {
	buf := p.buf
	if len(buf) < 2 {
		return frame{}, false, nil
	}
	first, second := buf[0], buf[1]
	if first&0x70 != 0 {
		return frame{}, false, protocolError("reserved bits")
	}
	opcode := int(first & 0x0F)
	fin := first&0x80 != 0
	masked := second&0x80 != 0
	length := int(second & 0x7F)
	offset := 2
	switch length {
	case 126:
		if len(buf) < 4 {
			return frame{}, false, nil
		}
		length = int(binary.BigEndian.Uint16(buf[2:4]))
		offset = 4
	case 127:
		if len(buf) < 10 {
			return frame{}, false, nil
		}
		wide := binary.BigEndian.Uint64(buf[2:10])
		if wide&(1<<63) != 0 {
			return frame{}, false, protocolError("length msb")
		}
		length = int(wide)
		offset = 10
	}
	if length > maxPayload {
		return frame{}, false, protocolError("frame too large")
	}
	maskLen := 0
	if masked {
		maskLen = 4
	}
	total := offset + maskLen + length
	if len(buf) < total {
		return frame{}, false, nil
	}
	payload := append([]byte(nil), buf[offset+maskLen:total]...)
	if masked {
		payload = applyMask(payload, buf[offset:offset+4])
	}
	p.buf = append([]byte(nil), buf[total:]...)
	control := opcode == opClose || opcode == opPing || opcode == opPong
	if control && (!fin || length > 125) {
		return frame{}, false, protocolError("bad control frame")
	}
	return frame{fin: fin, opcode: opcode, payload: payload}, true, nil
}

type byteStream struct {
	fragment bool
}

func (s *byteStream) push(fin bool, opcode int, payload []byte) ([]byte, error) {
	switch opcode {
	case opPing, opPong, opClose:
		return nil, protocolError("control frames are not stream bytes")
	case opText:
		return nil, protocolError("binary frames only")
	case opBinary:
		if s.fragment {
			return nil, protocolError("binary frame inside a fragment")
		}
		s.fragment = !fin
		return payload, nil
	case opContinuation:
		if !s.fragment {
			return nil, protocolError("unexpected continuation")
		}
		s.fragment = !fin
		return payload, nil
	default:
		return nil, protocolError(fmt.Sprintf("opcode %d", opcode))
	}
}
