const root = document.documentElement;
let theme = "system";
try {
  const saved = localStorage.getItem("ding-theme");
  if (["light", "dark", "system"].includes(saved)) theme = saved;
} catch {}
function applyTheme() {
  if (theme === "system") root.removeAttribute("data-theme");
  else root.dataset.theme = theme;
  const button = document.querySelector(".theme-toggle");
  button.hidden = false;
  button.textContent = `Theme: ${theme}`;
  button.setAttribute("aria-label", `Color theme: ${theme}. Change theme`);
}
applyTheme();
document.querySelector(".theme-toggle").addEventListener("click", () => {
  theme = ["system", "light", "dark"][
    (["system", "light", "dark"].indexOf(theme) + 1) % 3
  ];
  try {
    localStorage.setItem("ding-theme", theme);
  } catch {}
  applyTheme();
});
for (const window of document.querySelectorAll(".evidence-window")) {
  const controls = window.querySelector(".demo-controls");
  controls.hidden = false;
  const states = {
    firing: {
      badge: "Incident opened",
      title: "Three checks.\nThree server errors.",
      description: "The third matching response opened this incident.",
      observed: "503",
      count: "3 of 3",
      rule: "matching checks",
      event: "Firing",
      time: "00:00:10",
      code: "503",
      outcome: "Incident opened",
    },
    recovered: {
      badge: "Recovered",
      title: "Two new checks.\nBack to normal.",
      description: "Two nonmatching responses recovered the incident.",
      observed: "200",
      count: "2 of 2",
      rule: "recovery checks",
      event: "Recovered",
      time: "00:00:20",
      code: "200",
      outcome: "Incident recovered",
    },
  };
  controls.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-demo]");
    if (!button) return;
    for (const [key, value] of Object.entries(states[button.dataset.demo]))
      window.querySelector(`[data-demo-${key}]`).textContent = value;
    const rows = window.querySelectorAll(".observation");
    const history =
      button.dataset.demo === "firing"
        ? [
            ["00:00:00", "500", "Matching"],
            ["00:00:05", "502", "Matching"],
          ]
        : [
            ["00:00:10", "503", "Incident opened"],
            ["00:00:15", "200", "Recovery 1 of 2"],
          ];
    history.forEach((cells, i) =>
      cells.forEach((value, j) => (rows[i].children[j].textContent = value)),
    );
    window
      .querySelector("[data-demo-badge]")
      .classList.toggle("incident", button.dataset.demo === "firing");
    for (const b of controls.querySelectorAll("button"))
      b.setAttribute("aria-pressed", String(b === button));
  });
}
