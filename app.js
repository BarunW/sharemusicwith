const STORAGE_KEY = "connect-with-playlist-state-v1";
const SPOTIFY_PREVIEW_RESTORED_KEY = "connect-with-playlist-spotify-preview-restored-v1";
const RESPONSIVE_PLAYER_QUERY = "(max-width: 1040px)";

const themeCopy = {
  storybook:
    "Soft painted color, calm spacing, and a page that feels personal without becoming heavy.",
  cyber:
    "Rainy neon glass, sharp contrast, and a live-mix profile built for late-night sharing.",
  nature:
    "Layered forest texture, warm sunlight, and room for playlists to breathe.",
  minimal:
    "Quiet contrast, restrained texture, and a simple frame around the music.",
};

const defaultState = {
  user: {
    displayName: "Dream Listener",
    handle: "dreamlistener",
    bio: "A small public page for every playlist I want friends to hear.",
  },
  playlist: {
    title: "Weekend Rotation",
    links: [
      createSpotifyPreviewLink(),
      {
        id: crypto.randomUUID(),
        name: "Favorite videos",
        url: "https://www.youtube.com/playlist?list=PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj",
        platform: "YouTube",
        title: "YouTube playlist",
        embedUrl: "https://www.youtube.com/embed/videoseries?list=PLMC9KNkIncKtPzgY-5rmhvj7fax8fdxoj",
      },
    ],
  },
  theme: "storybook",
  density: "focused",
};

const state = loadState();
const responsivePlayerMedia = window.matchMedia(RESPONSIVE_PLAYER_QUERY);

const elements = {
  appShell: document.querySelector(".app-shell"),
  pageForm: document.querySelector("#pageForm"),
  displayName: document.querySelector("#displayName"),
  handle: document.querySelector("#handle"),
  bio: document.querySelector("#bio"),
  pageTitle: document.querySelector("#pageTitle"),
  playlistName: document.querySelector("#playlistName"),
  playlistUrl: document.querySelector("#playlistUrl"),
  addLink: document.querySelector("#addLink"),
  addSpotifyPreview: document.querySelector("#addSpotifyPreview"),
  viewOnly: document.querySelector("#viewOnly"),
  exitView: document.querySelector("#exitView"),
  playlistList: document.querySelector("#playlistList"),
  themeGrid: document.querySelector("#themeGrid"),
  phoneStage: document.querySelector(".phone-stage"),
  publicPage: document.querySelector("#publicPage"),
  playlistWebview: document.querySelector("#playlistWebview"),
  webviewPlatform: document.querySelector("#webviewPlatform"),
  webviewTitle: document.querySelector("#webviewTitle"),
  webviewFrame: document.querySelector("#webviewFrame"),
  closeWebview: document.querySelector("#closeWebview"),
  playlistInlinePlayer: document.querySelector("#playlistInlinePlayer"),
  inlinePlayerPlatform: document.querySelector("#inlinePlayerPlatform"),
  inlinePlayerTitle: document.querySelector("#inlinePlayerTitle"),
  inlinePlayerFrame: document.querySelector("#inlinePlayerFrame"),
  closeInlinePlayer: document.querySelector("#closeInlinePlayer"),
  previewName: document.querySelector("#previewName"),
  previewHandle: document.querySelector("#previewHandle"),
  previewBio: document.querySelector("#previewBio"),
  previewTitle: document.querySelector("#previewTitle"),
  avatar: document.querySelector("#avatar"),
  embedStack: document.querySelector("#embedStack"),
  themeNote: document.querySelector("#themeNote"),
  statusText: document.querySelector("#statusText"),
  playlistItemTemplate: document.querySelector("#playlistItemTemplate"),
  embedTemplate: document.querySelector("#embedTemplate"),
};

let isViewOnly = location.hash === "#view";
let activeWebviewLinkId = null;
let lastTouchY = 0;

hydrateForm();
render();
bindEvents();

function bindEvents() {
  elements.pageForm.addEventListener("input", (event) => {
    const target = event.target;
    if (!target.matches("input, textarea")) return;

    state.user.displayName = elements.displayName.value.trim();
    state.user.handle = normalizeHandle(elements.handle.value);
    state.user.bio = elements.bio.value.trim();
    state.playlist.title = elements.pageTitle.value.trim();
    persistAndRender();
  });

  elements.pageTitle.addEventListener("input", () => {
    state.playlist.title = elements.pageTitle.value.trim();
    persistAndRender();
  });

  elements.addLink.addEventListener("click", addPlaylistFromInput);
  elements.addSpotifyPreview.addEventListener("click", addSpotifyPreview);
  elements.embedStack.addEventListener("click", handleWebviewToggle);
  elements.closeWebview.addEventListener("click", closePlaylistWebview);
  elements.closeInlinePlayer.addEventListener("click", closePlaylistWebview);
  bindResponsivePlayerChange();
  window.addEventListener("wheel", handleViewWheel, { passive: false });
  window.addEventListener("touchstart", handleViewTouchStart, { passive: true });
  window.addEventListener("touchmove", handleViewTouchMove, { passive: false });
  window.addEventListener("keydown", handleViewKeydown);

  elements.playlistUrl.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      addPlaylistFromInput();
    }
  });

  elements.playlistList.addEventListener("click", (event) => {
    const button = event.target.closest("button[data-id]");
    if (!button) return;
    if (activeWebviewLinkId === button.dataset.id) activeWebviewLinkId = null;
    state.playlist.links = state.playlist.links.filter((link) => link.id !== button.dataset.id);
    persistAndRender();
  });

  elements.themeGrid.addEventListener("click", (event) => {
    const button = event.target.closest("[data-theme-option]");
    if (!button) return;
    state.theme = button.dataset.themeOption;
    persistAndRender();
  });

  document.querySelectorAll("[data-density]").forEach((button) => {
    button.addEventListener("click", () => {
      state.density = button.dataset.density;
      persistAndRender();
    });
  });

  elements.viewOnly.addEventListener("click", () => {
    isViewOnly = true;
    history.replaceState(null, "", "#view");
    renderEmbeds();
    renderViewMode();
  });

  elements.exitView.addEventListener("click", () => {
    isViewOnly = false;
    activeWebviewLinkId = null;
    history.replaceState(null, "", location.href.replace(/#.*$/, ""));
    renderEmbeds();
    renderViewMode();
  });

  window.addEventListener("hashchange", () => {
    isViewOnly = location.hash === "#view";
    if (!isViewOnly) activeWebviewLinkId = null;
    renderEmbeds();
    renderViewMode();
  });
}

function bindResponsivePlayerChange() {
  const handleChange = () => {
    renderViewMode();
  };

  if (responsivePlayerMedia.addEventListener) {
    responsivePlayerMedia.addEventListener("change", handleChange);
  } else {
    responsivePlayerMedia.addListener(handleChange);
  }
}

function addPlaylistFromInput() {
  const rawName = elements.playlistName.value.trim();
  const rawUrl = elements.playlistUrl.value.trim();
  if (!rawUrl) {
    setStatus("Paste a public playlist link first.");
    return;
  }

  const parsed = parsePlaylistLink(rawUrl);
  if (!parsed) {
    setStatus("That does not look like a valid public URL.");
    return;
  }

  parsed.name = rawName || getDefaultPlaylistName(parsed);
  state.playlist.links = [parsed, ...state.playlist.links];
  elements.playlistName.value = "";
  elements.playlistUrl.value = "";
  setStatus(`${parsed.name} added.`);
  persistAndRender();
}

function addSpotifyPreview() {
  const alreadyAdded = state.playlist.links.some((link) => isSpotifyPreview(link));
  if (alreadyAdded) {
    setStatus("Spotify preview is already on the page.");
    return;
  }

  state.playlist.links = [createSpotifyPreviewLink(), ...state.playlist.links];
  setStatus("Spotify preview added.");
  persistAndRender();
}

function hydrateForm() {
  elements.displayName.value = state.user.displayName;
  elements.handle.value = state.user.handle;
  elements.bio.value = state.user.bio;
  elements.pageTitle.value = state.playlist.title;
}

function render() {
  elements.appShell.dataset.theme = state.theme;
  elements.appShell.dataset.density = state.density;
  elements.publicPage.dataset.theme = state.theme;

  elements.previewName.textContent = state.user.displayName || "Your Name";
  elements.previewHandle.textContent = `@${state.user.handle || "yourhandle"}`;
  elements.previewBio.textContent = state.user.bio || "Add a short note about the playlists you share.";
  elements.previewTitle.textContent = state.playlist.title || "Name of the Playlist";
  elements.avatar.textContent = getInitials(state.user.displayName);
  elements.themeNote.textContent = themeCopy[state.theme];

  renderThemeControls();
  renderDensityControls();
  renderPlaylistList();
  renderEmbeds();
  renderViewMode();
}

function renderThemeControls() {
  document.querySelectorAll("[data-theme-option]").forEach((button) => {
    const active = button.dataset.themeOption === state.theme;
    button.classList.toggle("is-active", active);
    button.setAttribute("aria-checked", String(active));
  });
}

function renderDensityControls() {
  document.querySelectorAll("[data-density]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.density === state.density);
  });
}

function renderPlaylistList() {
  elements.playlistList.replaceChildren();

  state.playlist.links.forEach((link) => {
    const item = elements.playlistItemTemplate.content.firstElementChild.cloneNode(true);
    item.querySelector("strong").textContent = getPlaylistName(link);
    item.querySelector("span").textContent = `${link.platform} · ${link.url}`;
    item.querySelector("button").dataset.id = link.id;
    elements.playlistList.append(item);
  });
}

function renderEmbeds() {
  if (state.playlist.links.length === 0) {
    elements.embedStack.replaceChildren();
    const empty = document.createElement("article");
    empty.className = "music-block";
    empty.innerHTML = `
      <a class="fallback-link" href="#" aria-disabled="true">
        <strong>Songs will show here</strong>
        <span>Add a public playlist from YouTube, Spotify, Apple Music, SoundCloud, or any shareable URL.</span>
      </a>
    `;
    elements.embedStack.append(empty);
    return;
  }

  // Reconcile in place: moving a block (re-appending it) re-parents its
  // <iframe>, which forces the embed to reload. So drop removed blocks first,
  // then insert each block only when it isn't already at the right spot —
  // untouched blocks keep their loaded iframes.
  const nextIds = new Set(state.playlist.links.map((link) => link.id));
  Array.from(elements.embedStack.children).forEach((child) => {
    if (!child.dataset.linkId || !nextIds.has(child.dataset.linkId)) {
      child.remove();
    }
  });

  const existingBlocks = getExistingEmbedBlocks();
  let cursor = elements.embedStack.firstElementChild;
  state.playlist.links.forEach((link) => {
    const block = existingBlocks.get(link.id) || createEmbedBlock(link);
    updateEmbedBlock(block, link);
    if (block === cursor) {
      cursor = cursor.nextElementSibling;
    } else {
      elements.embedStack.insertBefore(block, cursor);
    }
  });
}

function getExistingEmbedBlocks() {
  return new Map(
    Array.from(elements.embedStack.children)
      .filter((child) => child.dataset.linkId)
      .map((child) => [child.dataset.linkId, child]),
  );
}

function createEmbedBlock(link) {
  const block = elements.embedTemplate.content.firstElementChild.cloneNode(true);
  block.dataset.linkId = link.id;
  return block;
}

function updateEmbedBlock(block, link) {
  const pill = block.querySelector(".platform-pill");
  const anchor = block.querySelector(".music-meta a");
  const toggle = block.querySelector(".webview-toggle");
  const target = block.querySelector(".embed-target");

  block.dataset.linkId = link.id;
  block.classList.toggle("spotify-block", link.platform === "Spotify");
  pill.textContent = link.platform;
  anchor.textContent = getPlaylistName(link);
  anchor.href = link.url;
  renderWebviewToggle(toggle, link);
  renderEmbedTarget(target, link);
}

function renderEmbedTarget(target, link) {
  const signature = getEmbedRenderSignature(link);
  if (
    target.dataset.renderKind === signature.kind &&
    target.dataset.renderSource === signature.source
  ) {
    if (signature.kind === "embed") {
      const iframe = target.querySelector("iframe");
      if (iframe) iframe.title = `${link.platform} playlist embed`;
    }
    return;
  }

  target.dataset.renderKind = signature.kind;
  target.dataset.renderSource = signature.source;

  if (signature.kind === "youtube") {
    target.replaceChildren(createYoutubeCard(link));
    return;
  }

  if (signature.kind === "embed") {
    mountIframe(target, createEmbedIframe(link, `${link.platform} playlist embed`));
    return;
  }

  target.replaceChildren(createFallbackLink(link));
}

function getEmbedRenderSignature(link) {
  if (isYoutubeFamily(link)) {
    return {
      kind: "youtube",
      source: [link.platform, link.url, link.embedUrl || ""].join("|"),
    };
  }

  if (canRenderEmbed(link)) {
    return { kind: "embed", source: link.embedUrl };
  }

  return { kind: "fallback", source: link.url };
}

function createEmbedIframe(link, title) {
  const iframe = document.createElement("iframe");
  iframe.loading = "lazy";
  iframe.scrolling = "no";
  iframe.allow = "autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture";
  iframe.referrerPolicy = "strict-origin-when-cross-origin";
  iframe.title = title;
  iframe.src = link.embedUrl;
  return iframe;
}

// mountIframe inserts an embed iframe into its container and shows a loading
// spinner (CSS .is-loading) until the iframe's load event fires, with a safety
// timeout so a failed embed never spins forever.
function mountIframe(container, iframe) {
  container.classList.add("is-loading");
  const done = () => container.classList.remove("is-loading");
  iframe.addEventListener("load", done, { once: true });
  setTimeout(done, 12000);
  container.replaceChildren(iframe);
}

function createFallbackLink(link) {
  const fallback = document.createElement("a");
  fallback.className = "fallback-link";
  fallback.href = link.url;
  fallback.target = "_blank";
  fallback.rel = "noreferrer";
  fallback.innerHTML = `
    <strong>Open public playlist</strong>
    <span>${getFallbackReason(link)}</span>
  `;
  return fallback;
}

function renderWebviewToggle(button, link) {
  const isActive = activeWebviewLinkId === link.id;
  const canOpenInline = canRenderEmbed(link);
  const action = isActive ? "Close player" : "Open player";

  button.dataset.linkId = link.id;
  button.textContent = canOpenInline ? action : "No player";
  button.disabled = !canOpenInline;
  button.classList.toggle("is-active", isActive);
  button.setAttribute("aria-pressed", String(isActive));
  button.setAttribute(
    "aria-label",
    canOpenInline ? `${action} for ${getPlaylistName(link)}` : `${getPlaylistName(link)} cannot play on this page`,
  );
  button.title = canOpenInline ? action : getFallbackReason(link);
}

function handleWebviewToggle(event) {
  const button = event.target.closest(".webview-toggle");
  if (!button) return;
  event.preventDefault();

  const link = findPlaylistLink(button.dataset.linkId);
  if (!link || !canRenderEmbed(link)) {
    setStatus("Inline playback is not available for this playlist.");
    return;
  }

  if (!isViewOnly) {
    setStatus("Use View Page to open playlist players.");
    return;
  }

  activeWebviewLinkId = activeWebviewLinkId === link.id ? null : link.id;
  renderEmbeds();
  renderViewMode();
}

function closePlaylistWebview() {
  activeWebviewLinkId = null;
  renderEmbeds();
  renderViewMode();
}

function renderWebview() {
  const activeLink = getActivePlayerLink();
  const canShowWebview = Boolean(activeLink && !usesResponsivePlayer());

  if (!canShowWebview) {
    elements.playlistWebview.hidden = true;
    elements.webviewFrame.replaceChildren();
    delete elements.webviewFrame.dataset.src;
    return;
  }

  elements.playlistWebview.hidden = false;
  elements.webviewPlatform.textContent = activeLink.platform;
  elements.webviewTitle.textContent = getPlaylistName(activeLink);

  if (elements.webviewFrame.dataset.src === activeLink.embedUrl) return;

  const iframe = document.createElement("iframe");
  iframe.loading = "lazy";
  iframe.allow = "autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture";
  iframe.referrerPolicy = "strict-origin-when-cross-origin";
  iframe.title = `${activeLink.platform} inline playlist player`;
  iframe.src = activeLink.embedUrl;

  elements.webviewFrame.dataset.src = activeLink.embedUrl;
  mountIframe(elements.webviewFrame, iframe);
  resetSplitScroll();
}

function renderResponsivePlayer() {
  const activeLink = getActivePlayerLink();
  const canShowResponsivePlayer = Boolean(activeLink && usesResponsivePlayer());

  if (!canShowResponsivePlayer) {
    elements.playlistInlinePlayer.hidden = true;
    elements.playlistInlinePlayer.classList.remove("spotify-block", "youtube-block", "apple-block");
    elements.inlinePlayerFrame.replaceChildren();
    delete elements.inlinePlayerFrame.dataset.src;
    return;
  }

  elements.playlistInlinePlayer.hidden = false;
  elements.playlistInlinePlayer.classList.toggle("spotify-block", activeLink.platform === "Spotify");
  elements.playlistInlinePlayer.classList.toggle("youtube-block", isYoutubeFamily(activeLink));
  elements.playlistInlinePlayer.classList.toggle("apple-block", activeLink.platform === "Apple Music");
  elements.inlinePlayerPlatform.textContent = activeLink.platform;
  elements.inlinePlayerTitle.textContent = getPlaylistName(activeLink);

  if (elements.inlinePlayerFrame.dataset.src === activeLink.embedUrl) return;

  const iframe = createEmbedIframe(activeLink, `${activeLink.platform} playlist player`);
  elements.inlinePlayerFrame.dataset.src = activeLink.embedUrl;
  mountIframe(elements.inlinePlayerFrame, iframe);
}

function getActivePlayerLink() {
  if (!isViewOnly) return null;
  const activeLink = findPlaylistLink(activeWebviewLinkId);
  return activeLink && canRenderEmbed(activeLink) ? activeLink : null;
}

function usesResponsivePlayer() {
  return responsivePlayerMedia.matches;
}

function handleViewWheel(event) {
  if (!isViewOnly || event.target.closest?.("#exitView")) return;
  if (activeWebviewLinkId && !event.target.closest?.("#publicPage")) return;
  event.preventDefault();
  scrollViewPlaylist(event.deltaY);
}

function handleViewTouchStart(event) {
  if (!isViewOnly || event.touches.length === 0) return;
  lastTouchY = event.touches[0].clientY;
}

function handleViewTouchMove(event) {
  if (!isViewOnly || event.touches.length === 0 || event.target.closest?.("#exitView")) return;
  if (activeWebviewLinkId && !event.target.closest?.("#publicPage")) return;
  event.preventDefault();

  const nextY = event.touches[0].clientY;
  scrollViewPlaylist(lastTouchY - nextY);
  lastTouchY = nextY;
}

function handleViewKeydown(event) {
  if (!isViewOnly || event.target.closest?.("#exitView")) return;
  if (activeWebviewLinkId && event.target.closest?.(".playlist-webview")) return;

  const scrollAmounts = {
    ArrowDown: 64,
    ArrowUp: -64,
    PageDown: elements.embedStack.clientHeight * 0.85,
    PageUp: elements.embedStack.clientHeight * -0.85,
    " ": elements.embedStack.clientHeight * 0.85,
    Home: -elements.embedStack.scrollHeight,
    End: elements.embedStack.scrollHeight,
  };

  if (!(event.key in scrollAmounts)) return;
  event.preventDefault();
  scrollViewPlaylist(scrollAmounts[event.key]);
}

function scrollViewPlaylist(deltaY) {
  elements.embedStack.scrollTop += deltaY;
}

function resetSplitScroll() {
  requestAnimationFrame(() => {
    if (elements.appShell.classList.contains("has-webview")) {
      elements.phoneStage.scrollTop = 0;
    }
  });
}

function canRenderEmbed(link) {
  if (!link.embedUrl) return false;
  return true;
}

function getFallbackReason(link) {
  return "This service does not provide a universal browser embed from only the public URL.";
}

function isYoutubeFamily(link) {
  return link.platform === "YouTube" || link.platform === "YouTube Music";
}

function findPlaylistLink(id) {
  return state.playlist.links.find((link) => link.id === id);
}

function createYoutubeCard(link) {
  const card = document.createElement("a");
  card.className = "platform-card youtube-card";
  card.href = link.url;
  card.target = "_blank";
  card.rel = "noreferrer";
  card.innerHTML = `
    <div class="platform-visual">
      <span class="play-badge"></span>
      <span class="queue-lines">
        <span></span>
        <span></span>
        <span></span>
      </span>
      <span class="open-arrow">↗</span>
    </div>
    <strong>${link.platform === "YouTube Music" ? "Open in YouTube Music" : "Open on YouTube"}</strong>
    <small>${link.embedUrl ? "Public video and playlist link" : "Public music playlist link"}</small>
  `;
  return card;
}

function renderViewMode() {
  if (!isViewOnly) activeWebviewLinkId = null;
  if (activeWebviewLinkId && !getActivePlayerLink()) activeWebviewLinkId = null;
  const hasActivePlayer = Boolean(getActivePlayerLink());
  elements.appShell.classList.toggle("view-only", isViewOnly);
  elements.appShell.classList.toggle("has-webview", hasActivePlayer);
  document.body.classList.toggle("view-only-body", isViewOnly);
  elements.viewOnly.setAttribute("aria-pressed", String(isViewOnly));
  renderResponsivePlayer();
  renderWebview();
}

function persistAndRender() {
  saveState(state);
  render();
}

function saveState(nextState) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(nextState));
}

function loadState() {
  try {
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY));
    const merged = mergeState(defaultState, stored);
    const didMigrate = migrateSavedLinks(merged);
    restoreSpotifyPreviewOnce(merged);
    if (didMigrate) saveState(merged);
    return merged;
  } catch {
    return structuredClone(defaultState);
  }
}

function restoreSpotifyPreviewOnce(nextState) {
  if (localStorage.getItem(SPOTIFY_PREVIEW_RESTORED_KEY) === "true") return;
  if (nextState.playlist.links.some((link) => link.platform === "Spotify")) return;

  nextState.playlist.links = [createSpotifyPreviewLink(), ...nextState.playlist.links];
  localStorage.setItem(SPOTIFY_PREVIEW_RESTORED_KEY, "true");
  saveState(nextState);
}

function mergeState(base, stored) {
  if (!stored || typeof stored !== "object") return structuredClone(base);
  return {
    user: { ...base.user, ...stored.user },
    playlist: {
      ...base.playlist,
      ...stored.playlist,
      links: normalizeLinks(
        Array.isArray(stored.playlist?.links) ? stored.playlist.links : base.playlist.links,
      ),
    },
    theme: stored.theme || base.theme,
    density: stored.density || base.density,
  };
}

function normalizeLinks(links) {
  return links.map((link) => ({
    ...link,
    id: link.id || crypto.randomUUID(),
    name: link.name || getDefaultPlaylistName(link),
  }));
}

function migrateSavedLinks(nextState) {
  let didMigrate = false;
  nextState.playlist.links = nextState.playlist.links.map((link) => {
    const migrated = migrateSavedAppleMusicLink(link);
    if (migrated !== link) didMigrate = true;
    return migrated;
  });
  return didMigrate;
}

function migrateSavedAppleMusicLink(link) {
  const parsed = parsePlaylistLink(link.url || "");
  if (!parsed || parsed.platform !== "Apple Music") return link;

  const oldName = link.name || "";
  const oldTitle = link.title || "";
  const migrated = {
    ...link,
    platform: parsed.platform,
    title: parsed.title,
    embedUrl: parsed.embedUrl,
  };

  if (shouldReplaceSavedAppleMusicName(oldName, oldTitle)) {
    migrated.name = getDefaultPlaylistName(migrated);
  }

  const didMigrate =
    migrated.name !== link.name ||
    migrated.platform !== link.platform ||
    migrated.title !== link.title ||
    migrated.embedUrl !== link.embedUrl;
  return didMigrate ? migrated : link;
}

function shouldReplaceSavedAppleMusicName(name, oldTitle) {
  return !name || name === "Apple Music link" || name === oldTitle;
}

function createSpotifyPreviewLink() {
  return {
    id: crypto.randomUUID(),
    name: "Today Top Hits",
    url: "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
    platform: "Spotify",
    title: "Spotify playlist",
    embedUrl: createSpotifyEmbedUrl("playlist", "37i9dQZF1DXcBWIGoYBM5M"),
  };
}

function isSpotifyPreview(link) {
  return link.platform === "Spotify" && link.url.includes("37i9dQZF1DXcBWIGoYBM5M");
}

function createSpotifyEmbedUrl(type, id) {
  return `https://open.spotify.com/embed/${type}/${encodeURIComponent(
    id,
  )}?utm_source=generator&theme=0`;
}

function createAppleMusicEmbedUrl(url) {
  const embedUrl = new URL(url.toString());
  embedUrl.host = "embed.music.apple.com";
  return embedUrl.toString();
}

function parsePlaylistLink(rawUrl) {
  let url;
  try {
    url = new URL(rawUrl);
  } catch {
    return null;
  }

  const host = url.hostname.replace(/^www\./, "");
  const pathParts = url.pathname.split("/").filter(Boolean);
  const link = {
    id: crypto.randomUUID(),
    url: url.toString(),
    platform: getPlatformName(host),
    title: getPlatformTitle(host, pathParts),
    embedUrl: "",
  };

  if (host.includes("music.youtube.com")) {
    const listId = url.searchParams.get("list");
    const videoId = url.searchParams.get("v");

    link.platform = "YouTube Music";
    link.title = listId ? "YouTube Music playlist" : "YouTube Music link";
    link.embedUrl =
      listId && !videoId
        ? ""
        : videoId
          ? `https://www.youtube.com/embed/${encodeURIComponent(videoId)}`
          : "";
  } else if (host.includes("youtube.com") || host.includes("youtu.be")) {
    const listId = url.searchParams.get("list");
    const videoId = host.includes("youtu.be") ? pathParts[0] : url.searchParams.get("v");

    link.platform = "YouTube";
    link.title = listId ? "YouTube playlist" : "YouTube video";
    link.embedUrl = listId
      ? `https://www.youtube.com/embed/videoseries?list=${encodeURIComponent(listId)}`
      : videoId
        ? `https://www.youtube.com/embed/${encodeURIComponent(videoId)}`
        : "";
  }

  if (host.includes("spotify.com")) {
    const supportedTypes = new Set(["playlist", "album", "track", "artist", "show", "episode"]);
    const [type, id] = pathParts;
    if (supportedTypes.has(type) && id) {
      link.platform = "Spotify";
      link.title = `Spotify ${type}`;
      link.embedUrl = createSpotifyEmbedUrl(type, id);
    }
  }

  if (host.includes("music.apple.com")) {
    link.platform = "Apple Music";
    link.title = url.pathname.includes("/playlist/") ? "Apple Music playlist" : "Apple Music link";
    link.embedUrl = createAppleMusicEmbedUrl(url);
  }

  if (host.includes("soundcloud.com")) {
    link.platform = "SoundCloud";
    link.title = "SoundCloud link";
  }

  return link;
}

function getPlatformName(host) {
  if (host.includes("music.youtube.com")) return "YouTube Music";
  if (host.includes("youtube") || host.includes("youtu.be")) return "YouTube";
  if (host.includes("spotify")) return "Spotify";
  if (host.includes("apple")) return "Apple Music";
  if (host.includes("soundcloud")) return "SoundCloud";
  return host.split(".").slice(-2, -1)[0] || "Playlist";
}

function getPlatformTitle(host, pathParts) {
  const platform = getPlatformName(host);
  const readableTail = decodeURIComponent(pathParts.at(-1) || "public link")
    .replace(/[-_]+/g, " ")
    .slice(0, 48);
  return `${platform} ${readableTail}`;
}

function getPlaylistName(link) {
  return link.name || getDefaultPlaylistName(link);
}

function getDefaultPlaylistName(link) {
  if (link.platform === "Spotify") return "Spotify playlist";
  if (link.platform === "YouTube Music") return "YouTube Music playlist";
  if (link.platform === "YouTube") return "YouTube playlist";
  return link.title || `${link.platform || "Public"} playlist`;
}

function normalizeHandle(value) {
  return value
    .trim()
    .replace(/^@/, "")
    .replace(/\s+/g, "")
    .replace(/[^\w.-]/g, "")
    .slice(0, 32);
}

function getInitials(name) {
  const letters = name
    .trim()
    .split(/\s+/)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return letters || "D";
}

function setStatus(message) {
  elements.statusText.textContent = message;
}
