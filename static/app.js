(() => {
  function hasModifierKey(ev) {
    return ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey;
  }

  function readEntries() {
    const el = document.getElementById("entries-data");
    if (!el) return null;
    try {
      return JSON.parse(el.textContent || "[]");
    } catch {
      return null;
    }
  }

  function bySlug(entries) {
    const map = new Map();
    for (const e of entries) {
      if (e && typeof e.slug === "string") map.set(e.slug, e);
    }
    return map;
  }

  function setText(el, value) {
    if (!el) return;
    el.textContent = value || "";
  }

  function setHidden(el, hidden) {
    if (!el) return;
    el.hidden = !!hidden;
  }

  function safeSetLink(a, href, text) {
    if (!a) return;
    if (href && typeof href === "string") {
      a.href = href;
      a.textContent = text || href;
      setHidden(a, false);
    } else {
      a.removeAttribute("href");
      a.textContent = "";
      setHidden(a, true);
    }
  }

  function formatMeta(type, date) {
    const t = (type || "").trim();
    const d = (date || "").trim();
    if (t && d) return `${t} · ${d}`;
    return t || d || "";
  }

  function truncateBody(body) {
    const s = body || "";
    if (s.length <= 1200) return s;
    return s.slice(0, 1200) + "…";
  }

  document.addEventListener("DOMContentLoaded", () => {
    const entries = readEntries();
    if (!entries) return;

    const entryMap = bySlug(entries);

    const preview = document.getElementById("entry-preview");
    if (!preview) return;

    const closeBtn = preview.querySelector("[data-preview-close]");
    const metaEl = preview.querySelector("[data-preview-meta]");
    const titleEl = preview.querySelector("[data-preview-title]");
    const summaryEl = preview.querySelector("[data-preview-summary]");
    const bodyEl = preview.querySelector("[data-preview-body]");
    const imageEl = preview.querySelector("[data-preview-image]");
    const sourceWrap = preview.querySelector("[data-preview-source-wrap]");
    const sourceLink = preview.querySelector("[data-preview-source-link]");
    const tagsWrap = preview.querySelector("[data-preview-tags]");
    const fullLink = preview.querySelector("[data-preview-link]");

    let selectedSlug = null;

    function clearSelectedCard() {
      const prev = document.querySelector(".card-link.is-selected");
      if (prev) prev.classList.remove("is-selected");
    }

    function setSelectedCard(slug) {
      clearSelectedCard();
      const el = document.querySelector(`.card-link[data-slug="${CSS.escape(slug)}"]`);
      if (el) el.classList.add("is-selected");
    }

    function closePreview({ updateHash } = { updateHash: true }) {
      selectedSlug = null;
      clearSelectedCard();
      setHidden(preview, true);
      preview.classList.remove("is-open");
      preview.setAttribute("aria-hidden", "true");
      document.body.classList.remove("preview-open");
      if (updateHash && location.hash) history.replaceState(null, "", location.pathname + location.search);
    }

    function renderSpaces(spaces) {
      if (!tagsWrap) return;
      tagsWrap.textContent = "";
      if (!Array.isArray(spaces) || spaces.length === 0) return;

      const frag = document.createDocumentFragment();
      for (const t of spaces) {
        const v = typeof t === "string" ? t : "";
        if (!v) continue;
        const span = document.createElement("span");
        span.className = "tag";
        span.textContent = v;
        frag.appendChild(span);
      }
      tagsWrap.appendChild(frag);
    }

    function openPreview(slug, { updateHash } = { updateHash: true }) {
      const e = entryMap.get(slug);
      if (!e) return;

      selectedSlug = slug;
      setSelectedCard(slug);

      setText(metaEl, formatMeta(e.type, e.published));
      setText(titleEl, e.title || e.slug || "");

      const summary = (e.summary || "").trim();
      if (summary) {
        setHidden(summaryEl, false);
        setText(summaryEl, summary);
      } else {
        setHidden(summaryEl, true);
        setText(summaryEl, "");
      }

      setText(bodyEl, truncateBody(e.body || ""));

      const imageURL = e.image_url || "";
      const imageAlt = e.image_alt || "";
      if (imageEl && imageURL) {
        imageEl.src = imageURL;
        imageEl.alt = imageAlt;
        setHidden(imageEl, false);
      } else if (imageEl) {
        imageEl.removeAttribute("src");
        imageEl.alt = "";
        setHidden(imageEl, true);
      }

      const srcURL = e.source_url || "";
      const srcTitle = e.source_title || "";
      const srcText = srcTitle || srcURL;
      if (srcURL) {
        setHidden(sourceWrap, false);
        safeSetLink(sourceLink, srcURL, srcText);
      } else {
        setHidden(sourceWrap, true);
        safeSetLink(sourceLink, "", "");
      }

      const permalinkHref = `/entries/${encodeURIComponent(slug)}`;
      safeSetLink(fullLink, permalinkHref, "Read full entry →");

      renderSpaces((e && Array.isArray(e.spaces) && e.spaces.length > 0) ? e.spaces : e.tags);

      setHidden(preview, false);
      preview.classList.add("is-open");
      preview.setAttribute("aria-hidden", "false");
      document.body.classList.add("preview-open");
      if (updateHash) location.hash = slug;
    }

    document.addEventListener("click", (ev) => {
      const a = ev.target instanceof Element ? ev.target.closest("a.card-link[data-slug]") : null;
      if (!a) return;
      if (hasModifierKey(ev) || ev.button !== 0) return;

      const slug = a.getAttribute("data-slug");
      if (!slug) return;
      if (!entryMap.has(slug)) return;

      ev.preventDefault();
      openPreview(slug, { updateHash: true });
    });

    if (closeBtn) {
      closeBtn.addEventListener("click", () => closePreview({ updateHash: true }));
    }

    document.addEventListener("keydown", (ev) => {
      if (ev.key === "Escape" && !preview.hidden) {
        ev.preventDefault();
        closePreview({ updateHash: true });
      }
    });

    window.addEventListener("hashchange", () => {
      const slug = (location.hash || "").replace(/^#/, "");
      if (!slug) {
        if (!preview.hidden) closePreview({ updateHash: false });
        return;
      }
      if (entryMap.has(slug)) openPreview(slug, { updateHash: false });
    });

    const initialSlug = (location.hash || "").replace(/^#/, "");
    if (initialSlug && entryMap.has(initialSlug)) {
      openPreview(initialSlug, { updateHash: false });
    }
  });
})();

