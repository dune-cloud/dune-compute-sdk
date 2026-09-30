// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

func present(value any, table func() error) error {
	if asJSON {
		return writeJSON(value)
	}
	return table()
}

func presentList(value any) error {
	switch typed := value.(type) {
	case *dune.ListInstancesResponse:
		return writeInstances(typed)
	case *dune.ListDisksResponse:
		return writeDisks(typed)
	case *dune.ListTemplatesResponse:
		return writeTemplates(typed)
	case *dune.ListSSHKeyPairsResponse:
		return writeKeys(typed)
	case *dune.ListNetworksResponse:
		return writeNetworks(typed)
	case *dune.ListZonesResponse:
		return writeZones(typed)
	case *dune.ListServiceOfferingsResponse:
		return writeServiceOfferings(typed)
	case *dune.ListDiskOfferingsResponse:
		return writeDiskOfferings(typed)
	case *dune.ListAPIsResponse:
		return writeAPIs(typed)
	default:
		return writeJSON(value)
	}
}

func writeTable(headers []string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := utf8.RuneCountInString(cell); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}
	if err := writeRow(headers, widths); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeRow(row, widths); err != nil {
			return err
		}
	}
	return nil
}

func writeRow(cells []string, widths []int) error {
	var b strings.Builder
	for i, cell := range cells {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(cell)
		if i+1 < len(cells) && i < len(widths) {
			pad := widths[i] - utf8.RuneCountInString(cell)
			if pad > 0 {
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
	}
	_, err := fmt.Fprintln(stdout, b.String())
	return err
}

func writeInstances(listed *dune.ListInstancesResponse) error {
	rows := make([][]string, 0, len(listed.Instances))
	for _, vm := range listed.Instances {
		if vm == nil {
			continue
		}
		rows = append(rows, []string{
			vm.Id, vm.Name, vm.State, vm.Ipaddress, vm.Zonename, vm.Serviceofferingname, vm.Templatename,
		})
	}
	return writeTable([]string{"ID", "NAME", "STATE", "IP", "ZONE", "OFFERING", "TEMPLATE"}, rows)
}

func writeDisks(listed *dune.ListDisksResponse) error {
	rows := make([][]string, 0, len(listed.Disks))
	for _, disk := range listed.Disks {
		if disk == nil {
			continue
		}
		rows = append(rows, []string{
			disk.Id, disk.Name, disk.State, formatBytes(disk.Size), disk.Type, disk.Vmname, disk.Zonename,
		})
	}
	return writeTable([]string{"ID", "NAME", "STATE", "SIZE", "TYPE", "VM", "ZONE"}, rows)
}

func writeTemplates(listed *dune.ListTemplatesResponse) error {
	rows := make([][]string, 0, len(listed.Templates))
	for _, template := range listed.Templates {
		if template == nil {
			continue
		}
		ready := "no"
		if template.Isready {
			ready = "yes"
		}
		rows = append(rows, []string{
			template.Id, template.Name, template.Format, template.Zonename, ready,
		})
	}
	return writeTable([]string{"ID", "NAME", "FORMAT", "ZONE", "READY"}, rows)
}

func writeKeys(listed *dune.ListSSHKeyPairsResponse) error {
	rows := make([][]string, 0, len(listed.SSHKeyPairs))
	for _, key := range listed.SSHKeyPairs {
		if key == nil {
			continue
		}
		rows = append(rows, []string{key.Name, key.Fingerprint})
	}
	return writeTable([]string{"NAME", "FINGERPRINT"}, rows)
}

func writeNetworks(listed *dune.ListNetworksResponse) error {
	rows := make([][]string, 0, len(listed.Networks))
	for _, network := range listed.Networks {
		if network == nil {
			continue
		}
		cidr := network.Cidr
		if cidr == "" {
			cidr = network.Networkcidr
		}
		rows = append(rows, []string{network.Id, network.Name, network.State, network.Type, cidr, network.Zonename})
	}
	return writeTable([]string{"ID", "NAME", "STATE", "TYPE", "CIDR", "ZONE"}, rows)
}

func writeZones(listed *dune.ListZonesResponse) error {
	rows := make([][]string, 0, len(listed.Zones))
	for _, zone := range listed.Zones {
		if zone == nil {
			continue
		}
		rows = append(rows, []string{zone.Id, zone.Name, zone.Networktype, zone.Allocationstate})
	}
	return writeTable([]string{"ID", "NAME", "NETWORK", "ALLOCATION"}, rows)
}

func writeServiceOfferings(listed *dune.ListServiceOfferingsResponse) error {
	rows := make([][]string, 0, len(listed.ServiceOfferings))
	for _, offering := range listed.ServiceOfferings {
		if offering == nil {
			continue
		}
		cpu, memory := "", ""
		if offering.Iscustomized {
			cpu, memory = "custom", "custom"
		} else {
			cpu = formatCount(offering.Cpunumber)
			memory = formatMB(offering.Memory)
		}
		rows = append(rows, []string{offering.Id, offering.Name, cpu, memory})
	}
	return writeTable([]string{"ID", "NAME", "CPU", "MEMORY"}, rows)
}

func writeDiskOfferings(listed *dune.ListDiskOfferingsResponse) error {
	rows := make([][]string, 0, len(listed.DiskOfferings))
	for _, offering := range listed.DiskOfferings {
		if offering == nil {
			continue
		}
		size := ""
		if offering.Disksize > 0 {
			size = fmt.Sprintf("%d GB", offering.Disksize)
		}
		custom := "no"
		if offering.Iscustomized {
			custom = "yes"
			if size == "" {
				size = "custom"
			}
		}
		rows = append(rows, []string{offering.Id, offering.Name, size, custom})
	}
	return writeTable([]string{"ID", "NAME", "SIZE", "CUSTOM"}, rows)
}

func writeAPIs(listed *dune.ListAPIsResponse) error {
	rows := make([][]string, 0, len(listed.Apis))
	for _, api := range listed.Apis {
		if api == nil {
			continue
		}
		async := "no"
		if api.Isasync {
			async = "yes"
		}
		rows = append(rows, []string{api.Name, async, shorten(api.Description, 72)})
	}
	return writeTable([]string{"NAME", "ASYNC", "DESCRIPTION"}, rows)
}

func writeJob(result *dune.QueryJobResponse) error {
	return writeTable([]string{"ID", "STATUS", "PROGRESS"}, [][]string{{
		result.JobID,
		jobStatusText(result.Jobstatus),
		fmt.Sprintf("%d%%", result.Jobprocstatus),
	}})
}

func jobStatusText(status int) string {
	switch status {
	case 0:
		return "pending"
	case 1:
		return "succeeded"
	case 2:
		return "failed"
	default:
		return fmt.Sprintf("%d", status)
	}
}

func writeResult(act action, value any) error {
	if err := finishProgress(act.doneLine(value)); err != nil {
		return err
	}
	key := privateKeyOf(value)
	if key == "" {
		return nil
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "private key:"); err != nil {
		return err
	}
	_, err := fmt.Fprintln(stdout, strings.TrimRight(key, "\n"))
	return err
}

func privateKeyOf(value any) string {
	object, err := objectOf(value)
	if err != nil {
		return ""
	}
	for _, record := range collectRecords(object) {
		if key := field(record, "privatekey"); key != "" {
			return key
		}
	}
	return ""
}

func labelOf(value any) string {
	object, err := objectOf(value)
	if err != nil {
		return ""
	}
	var id string
	for _, record := range collectRecords(object) {
		if name := field(record, "name"); name != "" {
			return name
		}
		if id == "" {
			id = field(record, "id")
		}
	}
	return id
}

func collectRecords(object map[string]any) []map[string]any {
	nested := nestedRecords(object)
	if field(object, "id", "name", "privatekey") == "" && len(nested) > 0 {
		return nested
	}
	if resourceLike(object) {
		return []map[string]any{object}
	}
	return nested
}

func nestedRecords(object map[string]any) []map[string]any {
	var records []map[string]any
	for _, value := range object {
		switch typed := value.(type) {
		case []any:
			for _, item := range typed {
				child, ok := item.(map[string]any)
				if ok && resourceLike(child) {
					records = append(records, child)
				}
			}
		case map[string]any:
			if resourceLike(typed) {
				records = append(records, typed)
				continue
			}
			records = append(records, nestedRecords(typed)...)
		}
	}
	return records
}

func resourceLike(object map[string]any) bool {
	for _, key := range []string{"id", "name", "state", "status", "displaytext", "privatekey", "fingerprint"} {
		if scalar(object[key]) != "" {
			return true
		}
	}
	success, ok := object["success"].(bool)
	return ok && success
}

func objectOf(value any) (map[string]any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not an object")
	}
	return object, nil
}

func field(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := scalar(object[key]); text != "" {
			return text
		}
	}
	return ""
}

func scalar(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func formatCount(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

func formatMB(n int) string {
	if n <= 0 {
		return ""
	}
	if n%1024 == 0 {
		return fmt.Sprintf("%d GB", n/1024)
	}
	return fmt.Sprintf("%d MB", n)
}

func formatBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(n)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if value == math.Trunc(value) {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func shorten(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	if limit < 2 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}
