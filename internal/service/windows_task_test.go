package service

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestRegisteredTaskOwnership(t *testing.T) {
	d, err := DefinitionFor("windows", t.TempDir(), t.TempDir(), "S-1-5-21-test", record())
	if err != nil {
		t.Fatal(err)
	}
	if !sameTask([]byte(d.Content), d.Content) {
		t.Fatal("own task rejected")
	}
	encoded := []byte{0xff, 0xfe}
	for _, word := range utf16.Encode([]rune(d.Content)) {
		encoded = binary.LittleEndian.AppendUint16(encoded, word)
	}
	if !sameTask(encoded, d.Content) {
		t.Fatal("UTF-16 task rejected")
	}
	for _, replacement := range [][2]string{
		{"<Arguments>", "<Arguments>other "},
		{"LeastPrivilege", "HighestAvailable"},
		{"S-1-5-21-test", "S-1-5-21-other"},
		{"</Actions>", "<ComHandler><ClassId>other</ClassId></ComHandler></Actions>"},
		{"</Exec>", "<WorkingDirectory>C:/foreign</WorkingDirectory></Exec>"},
	} {
		changed := strings.ReplaceAll(d.Content, replacement[0], replacement[1])
		if sameTask([]byte(changed), d.Content) {
			t.Fatal("accepted foreign task", replacement[0])
		}
	}
}
