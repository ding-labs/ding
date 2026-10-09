package watchcli

import (
	"fmt"
	"io"
	"os"

	"github.com/ding-labs/ding/internal/migrate"
	"github.com/spf13/cobra"
)

func migrateCommand() *cobra.Command {
	var config, out string
	var structured bool
	command := &cobra.Command{Use: "migrate --config LEGACY.yaml --out NEW_DIRECTORY", Short: "Convert supported legacy rules and report unsupported semantics without starting watches", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(config)
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read legacy config")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "read_failed", "cannot read legacy config")
		}
		result, err := migrate.Convert(raw)
		if err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "invalid_legacy_config", err.Error())
		}
		if err := migrate.Write(out, result); err != nil {
			return Fail(cmd.ErrOrStderr(), structured, "migration_write_failed", err.Error())
		}
		if structured {
			if err := Write(cmd.OutOrStdout(), result.Report); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Converted %d rules; %d unsupported. Review %s/report.json before applying. State starts fresh.\n", result.Report.Converted, result.Report.Unsupported, out)
		}
		if result.Report.Unsupported > 0 {
			return Fail(cmd.ErrOrStderr(), structured, "partial_conversion", "unsupported rules were not emitted; review the per-rule report")
		}
		return nil
	}}
	command.Flags().StringVar(&config, "config", "ding.yaml", "legacy configuration path")
	command.Flags().StringVar(&out, "out", "", "new output directory")
	_ = command.MarkFlagRequired("out")
	command.Flags().BoolVar(&structured, "json", false, "emit a versioned migration report")
	return command
}
