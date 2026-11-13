# Tell-Tail dashboard images

`dashboard.png` is an unmodified full-page browser screenshot of the published
Tell-Tail 0.2.0 Apple Silicon binary. It uses synthetic INFO/WARN/ERROR records
on stdin, a five-second rolling window, dark theme, a 1200 × 980 browser viewport,
and the dashboard's Freeze display control. No real logs or customer data appear.

Source: https://github.com/ding-labs/tell-tail/releases/tag/v0.2.0 (MIT).
Archive SHA-256: `4b337ab47528246bd87501c7a8465e7c05e67cdfc0b730ba3bf5e8129913e252`.

Keep the visible sample-data caption when reusing it. Replace the screenshot when
changing the advertised release; do not substitute an unreleased dashboard.


## MCP dashboard

`mcp-dashboard.png` is an unmodified full-page screenshot of the published
Tell-Tail 0.4.0 Apple Silicon binary running its synthetic MCP demo. No client
logs or customer data are read. The visible demo banner and caption must be
retained when reusing it.

Source: https://github.com/ding-labs/tell-tail/releases/tag/v0.4.0 (MIT).
Source commit: `09df02f7c233b819b94c08c6229b2ea0934ec206`.
Executable SHA-256: `f55527f573d5a5600967bebddf432a24be4be013f77133b84dc45c2d0a447c88`.
Captured November 12, 2026 (Pacific), Chromium, dark theme, 1440 × 1100 viewport.
The full-page PNG is 1440 × 1602 pixels.

To reproduce, run `tell-tail mcp --demo --open`. After the third failure group
appears (about ten seconds), select **Pause display**, set **Category** to
**Failures**, and collapse **Sources and coverage**. Capture the full page.
The screenshot shows a recovered authentication failure alongside timeout and
transport failures, using the released collector, parsers, and dashboard.
