(() => {
  const KEY = "killy-lang";
  const buttons = document.querySelectorAll(".lang button");
  const apply = (lang) => {
    document.documentElement.lang = lang;
    for (const b of buttons) b.setAttribute("aria-pressed", String(b.dataset.lang === lang));
  };
  let stored = null;
  try { stored = localStorage.getItem(KEY); } catch {}
  const browserJa = (navigator.languages || [navigator.language]).some((l) => /^ja\b/i.test(l || ""));
  apply(stored === "en" || stored === "ja" ? stored : browserJa ? "ja" : "en");
  for (const b of buttons) {
    b.addEventListener("click", () => {
      apply(b.dataset.lang);
      try { localStorage.setItem(KEY, b.dataset.lang); } catch {}
    });
  }
})();
