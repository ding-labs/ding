const surfaces = {
  website: { title: 'An unmistakable first impression.', description: 'The original green leads. Violet frames the product; warm neutrals carry the story.', context: 'Actual website build. Product illustration uses sample data.' },
  console: { title: 'The signal, without the noise.', description: 'A quieter working surface. The same bell, clear actions, and separate operational states.', context: 'Actual console build with an isolated sample watch. No production data.' },
  docs: { title: 'Room to read. A clear next step.', description: 'Warm pages, readable code, pine links, and the original bell at the top of every guide.', context: 'Actual MkDocs build: create your first watch. Source preview instructions.' }
};
let surface = 'website';
let theme = 'light';
function render() {
  document.documentElement.dataset.theme = theme;
  for (const b of document.querySelectorAll('[data-surface]')) b.setAttribute('aria-pressed', String(b.dataset.surface === surface));
  for (const b of document.querySelectorAll('[data-theme-choice]')) b.setAttribute('aria-pressed', String(b.dataset.themeChoice === theme));
  document.querySelector('#surface-title').textContent = surfaces[surface].title;
  document.querySelector('#surface-description').textContent = surfaces[surface].description;
  document.querySelector('#capture-context').textContent = surfaces[surface].context;
  const image = document.querySelector('#preview-image');
  image.src = `previews/${surface}-${theme}.png`;
  image.alt = `${surface} design draft in the ${theme} theme, using the restored bell and original green`;
  document.querySelector('.capture').href = image.src;
  document.querySelector('#full-size').href = image.src;
}
for (const b of document.querySelectorAll('[data-surface]')) b.addEventListener('click', () => { surface = b.dataset.surface; render(); });
for (const b of document.querySelectorAll('[data-theme-choice]')) b.addEventListener('click', () => { theme = b.dataset.themeChoice; render(); });
