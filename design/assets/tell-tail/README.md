# Tell-Tail dashboard images

## Logo and log dashboard

`logo.svg` is the current upright-hook mark copied unchanged from the published
Tell-Tail 0.4.0 source (`web/logo.svg`). The website header pairs it with the
same system-font family, 650 weight, 26px size and -.8px letter spacing as the
released dashboard wordmark.

`dashboard.png` is an unmodified full-page screenshot of the published 0.4.0
Apple Silicon binary. Run `tell-tail --demo --window 5 --output web --open`,
wait for the error phase (about twenty seconds), and select **Freeze display**.
The capture uses dark theme and a 1200 × 980 viewport; the full image is
1200 × 2129 pixels. Captured November 12, 2026 (Pacific). No real logs or
customer data appear.

Source: https://github.com/ding-labs/tell-tail/releases/tag/v0.4.0 (MIT).
Source commit: `09df02f7c233b819b94c08c6229b2ea0934ec206`.
Executable SHA-256: `f55527f573d5a5600967bebddf432a24be4be013f77133b84dc45c2d0a447c88`.

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

## AI observability homepage captures

Captured November 21, 2026 in Chromium at 1.5× pixel density, dark appearance, from Tell-Tail's built-in synthetic demos. The dates, counts and local execution labels will vary between runs. Keep the demo and availability captions with these images.

| Image | Product state and framing |
| --- | --- |
| `activity.png` | AI Activity source preview. At 1240 × 790, open the Activity feed and select the failed `demo-deploy · status` row. Capture the workspace with the evidence inspector open. |
| `tool-result.png` | Same selected result. At 390 × 820, capture the upper 386 CSS pixels of the mobile inspector, including the source-reported duration. |
| `activity-mobile.png` | Same mobile session. Close the inspector and capture the visible tool rows. |
| `mcp-recovery.png` | Released MCP dashboard. Once the calendar OAuth group reports recovery, pause the display. At a 510-pixel viewport, capture the failure group with its own recovery time. |
| `log-throughput.png` | Released log dashboard. Freeze after the first silent phase and the return of traffic, then capture the Throughput panel from an 1100-pixel viewport. |

### AI Activity

Build from [the captured source revision](https://github.com/ding-labs/tell-tail/tree/b1f381fcece43d36397b46f6c2b36e992fdddd0d) with Rust 1.88 or later. From that checkout, run the demo.

```sh
cargo run --locked -- desktop --demo --open
```

The demo seeds the tool result with a 194 ms duration. Real Codex tool-result capture requires opting into the exporter. The screenshot uses the demo only.

### MCP and application logs

Install the [published 0.4.0 binary](https://github.com/ding-labs/tell-tail/releases/tag/v0.4.0). The Homebrew Apple Silicon executable used here has SHA-256 `f55527f573d5a5600967bebddf432a24be4be013f77133b84dc45c2d0a447c88`. Run either demo.

```sh
tell-tail mcp --demo --open
tell-tail --demo --window 5 --output web --open
```
