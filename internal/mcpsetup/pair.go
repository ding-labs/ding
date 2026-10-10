// Package mcpsetup provisions scoped grants through explicit local admin access.
package mcpsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ding-labs/ding/internal/mcpclient"
	"github.com/ding-labs/ding/internal/mcpconfig"
)

type PairOptions struct {
	State, Config, Name          string
	Days                         int
	Manage, Retry                bool
	SecretRefs, CommandRevisions []string
}

func AdminCall(ctx context.Context, state, method, suffix string, body any) (string, json.RawMessage, error) {
	var connection struct {
		URL string `json:"url"`
	}
	var tokens struct {
		Admin string `json:"admin"`
	}
	if mcpconfig.ReadPrivateJSON(filepath.Join(state, "connection.json"), &connection, false) != nil || mcpconfig.ReadPrivateJSON(filepath.Join(state, "tokens.json"), &tokens, false) != nil || tokens.Admin == "" {
		return "", nil, mcpconfig.ErrConfig
	}
	origin, err := mcpconfig.Endpoint(connection.URL, false)
	if err != nil {
		return "", nil, err
	}
	if suffix != "" {
		segment, err := mcpclient.Segment(suffix)
		if err != nil {
			return "", nil, err
		}
		suffix = "/" + segment
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", nil, mcpconfig.ErrConfig
	}
	req, err := http.NewRequestWithContext(ctx, method, origin+"/v1/integrations/grants"+suffix, bytes.NewReader(b))
	if err != nil {
		return "", nil, mcpconfig.ErrConfig
	}
	req.Header.Set("Authorization", "Bearer "+tokens.Admin)
	req.Header.Set("Content-Type", "application/json")
	client := mcpclient.HTTPClient()
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return "", nil, mcpclient.Fail("pairing_failed", "Ding could not complete the grant operation")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", nil, mcpclient.Fail("pairing_failed", "Ding could not complete the grant operation")
	}
	var envelope struct {
		APIVersion string          `json:"apiVersion"`
		Data       json.RawMessage `json:"data"`
		Error      json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.APIVersion != "ding.ing/v1alpha1" || len(envelope.Data) == 0 || len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return "", nil, mcpclient.Fail("pairing_failed", "Ding could not complete the grant operation")
	}
	return origin, envelope.Data, nil
}

func Pair(ctx context.Context, o PairOptions, out io.Writer) error {
	if o.State == "" || o.Config == "" || o.Days < 1 || o.Days > 365 {
		return mcpconfig.ErrConfig
	}
	f, err := mcpconfig.CreatePrivate(o.Config)
	if err != nil {
		return err
	}
	committed := false
	grantID := ""
	defer func() {
		f.Close()
		if !committed {
			_ = os.Remove(o.Config)
			if grantID != "" {
				_, _, err := AdminCall(context.WithoutCancel(ctx), o.State, http.MethodDelete, grantID, nil)
				if err != nil {
					fmt.Fprintln(out, "Pairing failed after grant creation; inspect local grants and revoke the unused grant.")
				}
			}
		}
	}()
	scopes := []string{"inspect", "preview"}
	if o.Manage {
		scopes = append(scopes, "manage")
	}
	if o.Retry {
		scopes = append(scopes, "retry")
	}
	origin, raw, err := AdminCall(ctx, o.State, http.MethodPost, "", map[string]any{"name": o.Name, "days": o.Days, "scopes": scopes, "commandRevisions": o.CommandRevisions, "secretRefs": o.SecretRefs})
	if err != nil {
		return err
	}
	var result struct {
		Token string `json:"token"`
		Grant struct {
			ID        string `json:"id"`
			ExpiresAt string `json:"expiresAt"`
		} `json:"grant"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return mcpconfig.ErrConfig
	}
	grantID = result.Grant.ID
	connection := mcpconfig.Connection{DaemonURL: origin, Token: result.Token, GrantID: grantID}
	if err := connection.Validate(); err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(connection); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	committed = true
	fmt.Fprintf(out, "Paired %s. Permissions: %v. Expires %s.\nPrivate connection saved. The credential was not printed.\n", o.Name, scopes, result.Grant.ExpiresAt)
	return nil
}
