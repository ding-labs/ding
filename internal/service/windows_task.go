package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"unicode/utf16"
)

// Scheduler adds registration metadata and default settings, so compare the
// executable, full arguments, run identity and action count semantically.
func sameTask(actual []byte, expected string) bool {
	if len(actual) > 128<<10 {
		return false
	}
	if bytes.HasPrefix(actual, []byte{0xff, 0xfe}) || bytes.HasPrefix(actual, []byte{0xfe, 0xff}) {
		var order binary.ByteOrder = binary.LittleEndian
		if actual[0] == 0xfe {
			order = binary.BigEndian
		}
		if len(actual)%2 != 0 {
			return false
		}
		words := make([]uint16, (len(actual)-2)/2)
		for i := range words {
			words[i] = order.Uint16(actual[2+i*2:])
		}
		actual = []byte(string(utf16.Decode(words)))
	}
	// XML is UTF-8 after decoding; discard only the optional declaration.
	decode := func(data []byte) (taskIdentity, error) {
		data = bytes.TrimSpace(data)
		if bytes.HasPrefix(data, []byte("<?xml")) {
			end := bytes.Index(data, []byte("?>"))
			if end < 0 {
				return taskIdentity{}, fmt.Errorf("invalid declaration")
			}
			data = data[end+2:]
		}
		var task taskIdentity
		err := xml.Unmarshal(data, &task)
		return task, err
	}
	a, err := decode(actual)
	b, expectedErr := decode([]byte(expected))
	if err != nil || expectedErr != nil || len(a.Actions.Items) != 1 || len(b.Actions.Items) != 1 || len(a.Principals) != 1 || len(b.Principals) != 1 {
		return false
	}
	return a.Actions.Items[0] == b.Actions.Items[0] && a.Principals[0] == b.Principals[0]
}

type taskIdentity struct {
	XMLName xml.Name `xml:"Task"`
	Actions struct {
		Items []taskAction `xml:",any"`
	} `xml:"Actions"`
	Principals []taskPrincipal `xml:"Principals>Principal"`
}
type taskAction struct {
	XMLName          xml.Name
	Command          string `xml:"Command"`
	Arguments        string `xml:"Arguments"`
	WorkingDirectory string `xml:"WorkingDirectory"`
}
type taskPrincipal struct {
	User  string `xml:"UserId"`
	Logon string `xml:"LogonType"`
	Level string `xml:"RunLevel"`
}

func (m Manager) checkRegisteredTask(ctx context.Context) error {
	data, err := m.Run(ctx, "schtasks", "/Query", "/TN", m.Definition.Name, "/XML")
	if err != nil {
		return fmt.Errorf("cannot inspect registered task: %w", err)
	}
	if !sameTask(data, m.Definition.Content) {
		return fmt.Errorf("registered task differs from this installation; inspect it in Task Scheduler")
	}
	return nil
}
