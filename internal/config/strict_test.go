package config

import (
	"strings"
	"testing"
)

func TestStrictConfig(t *testing.T) {
	for _, input := range []string{
		"server: {prt: 8080}", "server: {port: -2}", "server: {max_body_bytes: -2}", "server: {max_buffer_size: -2}", "server: {read_timeout: -1s}",
		"rules: [{name: a, condition: 'value > 1'}, {name: a, condition: 'value > 1'}]",
		"notifiers: {a: {type: webhook, url: 'file:///tmp/a'}}", "notifiers: {a: {type: webhook, url: 'https://example.com', max_attempts: -1}}",
		"server: {listen: 0.0.0.0}", "server: {port: 70000}", "server: {}\n---\nserver: {}", "server: {port: 1, port: 2}",
	} {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
}
func TestEnvironmentValueCannotInjectYAML(t *testing.T) {
	payload := "hello\nserver:\n  listen: 0.0.0.0\n# \" :"
	t.Setenv("DING_TEST_VALUE", payload)
	cfg, err := Parse([]byte("rules:\n- name: example\n  condition: value > 1\n  message: ${DING_TEST_VALUE}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].Message != payload || cfg.Server.Listen != "127.0.0.1" {
		t.Fatal("injected config")
	}
	_, err = Parse([]byte("server: {admin_token: '${DING_MISSING_VALUE}'}"))
	if err == nil || !strings.Contains(err.Error(), "unset env") {
		t.Fatal(err)
	}
}
