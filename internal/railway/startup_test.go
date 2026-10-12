package railway

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func environment() map[string]string {
	return map[string]string{"DING_PUBLIC_URL": "https://cloud.ding.ing", "DING_DATA_KEY_BASE64": base64.StdEncoding.EncodeToString(make([]byte, 32)), "PORT": "8080"}
}

func TestConfigurationFailsClosed(t *testing.T) {
	for name, change := range map[string]map[string]string{
		"invalid push": {"DING_PUSH_JSON": `{"hostConcurrent":-1}`}, "unknown push field": {"DING_PUSH_JSON": `{"disable":true}`}, "oversized store": {"DING_CAPACITY_JSON": `{"maxStoreBytes":1073741824}`},
		"missing key": {"DING_DATA_KEY_BASE64": ""}, "wrong key length": {"DING_DATA_KEY_BASE64": "YQ=="},
		"plaintext origin": {"DING_PUBLIC_URL": "http://cloud.ding.ing"}, "missing port": {"PORT": ""}, "privileged port": {"PORT": "80"}, "operator conflict": {"PORT": "8788"},
		"path escape": {"DING_STATE_NAME": "../etc"}, "backup directory": {"DING_STATE_NAME": "backups"}, "unknown mode": {"DING_MODE": "recover"},
		"implicit bootstrap": {"DING_BOOTSTRAP_PASSWORD": "a long password"}, "incomplete init": {"DING_MODE": "init"},
	} {
		t.Run(name, func(t *testing.T) {
			env := environment()
			for k, v := range change {
				env[k] = v
			}
			if _, _, _, err := FromEnv(func(k string) string { return env[k] }); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	env := environment()
	s, key, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if s.Config.Hosting != "cloud" || s.Config.Deployment != "railway" || len(s.Config.LoginMethods) != 1 || s.Config.LoginMethods[0] != "password" || *s.Config.EnrollmentLimit != 0 || s.Config.MCP != nil || len(key) != 32 {
		t.Fatal("unsafe hosting profile")
	}
}

func TestVolumeMustActuallyBeMounted(t *testing.T) {
	for _, info := range []string{"", "1 2 3 / /data-other rw - ext4 disk rw", "1 2 3 / / rw - overlay disk rw"} {
		if Mounted(info) {
			t.Fatal(info)
		}
	}
	if !Mounted("1 2 3 / /data rw - ext4 disk rw") {
		t.Fatal("real mount rejected")
	}
}

func TestMaintenanceIsReadyButNeverHealthyOrServingAccounts(t *testing.T) {
	env := environment()
	s, _, _, _ := FromEnv(func(k string) string { return env[k] })
	h := maintenanceHandler(s.Config)
	for _, tc := range []struct {
		host, path string
		want       int
	}{
		{"cloud.ding.ing", "/readyz", 200}, {"healthcheck.railway.app", "/readyz", 200},
		{"cloud.ding.ing", "/healthz", 503}, {"cloud.ding.ing", "/v1/watches", 503}, {"cloud.ding.ing", "/ui/", 503},
		{"healthcheck.railway.app", "/ui/", 403}, {"evil.example", "/readyz", 403},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil))
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
}

func TestPushConfigurationPreservesCloudProfile(t *testing.T) {
	env := environment()
	env["DING_PUSH_JSON"] = `{"disabled":true,"producerRate":2}`
	env["DING_CAPACITY_JSON"] = `{"maxWatches":18}`
	s, _, _, err := FromEnv(func(k string) string { return env[k] })
	if err != nil || !s.Config.Push.Disabled || s.Config.Push.ProducerRate != 2 || s.Config.Capacity.MaxWatches != 18 || s.Config.Hosting != "cloud" {
		t.Fatal(s, err)
	}
}
