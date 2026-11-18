(() => {
  "use strict";

  const elements = {
    loginButton: document.getElementById("login-button"),
    userArea: document.getElementById("user-area"),
    userEmail: document.getElementById("user-email"),
    logoutButton: document.getElementById("logout-button"),
    newObjectButton: document.getElementById("new-object-button"),
    searchInput: document.getElementById("search-input"),
    categoryFilter: document.getElementById("category-filter"),
    tagFilter: document.getElementById("tag-filter"),
    clearFilters: document.getElementById("clear-filters"),
    status: document.getElementById("status"),
    catalogSection: document.getElementById("catalog-section"),
    loading: document.getElementById("loading"),
    emptyState: document.getElementById("empty-state"),
    emptyMessage: document.getElementById("empty-message"),
    objectGrid: document.getElementById("object-grid"),
    pagination: document.getElementById("pagination"),
    prevPage: document.getElementById("prev-page"),
    nextPage: document.getElementById("next-page"),
    pageInfo: document.getElementById("page-info"),
    categoryList: document.getElementById("category-list"),
    newCategoryButton: document.getElementById("new-category-button"),
    loginDialog: document.getElementById("login-dialog"),
    loginForm: document.getElementById("login-form"),
    loginEmail: document.getElementById("login-email"),
    loginPassword: document.getElementById("login-password"),
    loginError: document.getElementById("login-error"),
    objectDialog: document.getElementById("object-dialog"),
    objectForm: document.getElementById("object-form"),
    objectDialogTitle: document.getElementById("object-dialog-title"),
    objectId: document.getElementById("object-id"),
    objectName: document.getElementById("object-name"),
    objectDescription: document.getElementById("object-description"),
    objectCategory: document.getElementById("object-category"),
    objectPhoto: document.getElementById("object-photo"),
    objectPhotoPreviewWrap: document.getElementById("object-photo-preview-wrap"),
    objectPhotoPreview: document.getElementById("object-photo-preview"),
    objectRemovePhoto: document.getElementById("object-remove-photo"),
    objectImages: document.getElementById("object-images"),
    objectImagesStrip: document.getElementById("object-images-strip"),
    objectTags: document.getElementById("object-tags"),
    objectError: document.getElementById("object-error"),
    objectSave: document.getElementById("object-save"),
busyDialog: document.getElementById("busy-dialog"),
    busyForm: document.getElementById("busy-form"),
    busyDialogTitle: document.getElementById("busy-dialog-title"),
    busyObjectId: document.getElementById("busy-object-id"),
    busyUntil: document.getElementById("busy-until"),
    busyError: document.getElementById("busy-error"),
    busySave: document.getElementById("busy-save"),
    reserveDialog: document.getElementById("reserve-dialog"),
    reserveForm: document.getElementById("reserve-form"),
    reserveDialogTitle: document.getElementById("reserve-dialog-title"),
    reserveObjectId: document.getElementById("reserve-object-id"),
    reserveFrom: document.getElementById("reserve-from"),
    reserveUntil: document.getElementById("reserve-until"),
    reserveError: document.getElementById("reserve-error"),
    reserveSave: document.getElementById("reserve-save"),
    reservationsList: document.getElementById("reservations-list"),
    categoryDialog: document.getElementById("category-dialog"),
    categoryForm: document.getElementById("category-form"),
    categoryDialogTitle: document.getElementById("category-dialog-title"),
    categoryId: document.getElementById("category-id"),
    categoryName: document.getElementById("category-name"),
    categoryError: document.getElementById("category-error"),
    categorySave: document.getElementById("category-save"),
    detailDialog: document.getElementById("detail-dialog"),
    detailLoading: document.getElementById("detail-loading"),
    detailContent: document.getElementById("detail-content"),
    detailTitle: document.getElementById("detail-title"),
    detailBadges: document.getElementById("detail-badges"),
    detailStage: document.getElementById("detail-stage"),
    detailMainImage: document.getElementById("detail-main-image"),
    detailPrev: document.getElementById("detail-prev"),
    detailNext: document.getElementById("detail-next"),
    detailThumbs: document.getElementById("detail-thumbs"),
    detailCaption: document.getElementById("detail-caption"),
    detailDescription: document.getElementById("detail-description"),
    detailTags: document.getElementById("detail-tags"),
    detailDates: document.getElementById("detail-dates"),
    detailEdit: document.getElementById("detail-edit"),
    views: document.querySelectorAll("[data-view]"),
    viewLinks: document.querySelectorAll("[data-view-link]"),
    activeFilters: document.getElementById("active-filters"),
    categoryCardsSection: document.getElementById("category-cards-section"),
    categoryCards: document.getElementById("category-cards"),
    resultsTitle: document.getElementById("results-title"),
    resultsCount: document.getElementById("results-count"),
    statObjects: document.getElementById("stat-objects"),
    statCategories: document.getElementById("stat-categories"),
  };

  const state = {
    user: null,
    categories: [],
    objects: [],
    total: 0,
    limit: 12,
    offset: 0,
    filters: { q: "", category: "", tag: "" },
    requestId: 0,
    editingObjectId: null,
    editingObjectImages: [],
    pendingUploads: [],
    pendingPhoto: null,
    editingBusyId: null,
    editingCategoryId: null,
    detailObjectId: null,
    detailName: "",
    detailImages: [],
    detailIndex: 0,
    detailRequestId: 0,
    view: "",
    categoryStats: new Map(),
    statsRequestId: 0,
  };

  let statusTimer;
  let photoObjectURL = "";
  let pendingImageURLs = [];

  function el(tag, className, text) {
    const node = document.createElement(tag);
    if (className) {
      node.className = className;
    }
    if (text !== undefined && text !== null) {
      node.textContent = String(text);
    }
    return node;
  }

  function option(value, label) {
    const node = el("option", "", label);
    node.value = value;
    return node;
  }

  function debounce(fn, delay) {
    let timeout;
    return (...args) => {
      clearTimeout(timeout);
      timeout = setTimeout(() => fn(...args), delay);
    };
  }

  async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const init = {
      method: options.method || "GET",
      credentials: "same-origin",
      headers,
    };
    if (options.body !== undefined) {
      if (options.body instanceof FormData) {
        init.body = options.body;
      } else {
        headers.set("Content-Type", "application/json");
        init.body = JSON.stringify(options.body);
      }
    }

    let response;
    try {
      response = await fetch(path, init);
    } catch (cause) {
      const error = new Error("Errore di rete. Il server è in esecuzione?");
      error.cause = cause;
      throw error;
    }

    if (response.status === 204) {
      return null;
    }

    const text = await response.text();
    let payload = null;
    if (text) {
      try {
        payload = JSON.parse(text);
      } catch {
        payload = null;
      }
    }

    if (!response.ok) {
      const message = payload?.error?.message || `Richiesta non riuscita (${response.status}).`;
      const error = new Error(message);
      error.status = response.status;
      error.code = payload?.error?.code;
      throw error;
    }
    return payload;
  }

  function showStatus(message, kind = "info", timeout = 5000) {
    clearTimeout(statusTimer);
    elements.status.textContent = message;
    elements.status.dataset.kind = kind;
    elements.status.hidden = false;
    if (timeout > 0) {
      statusTimer = setTimeout(() => {
        elements.status.hidden = true;
      }, timeout);
    }
  }

  function showFormError(target, message) {
    target.textContent = message;
    target.hidden = false;
  }

  function hideFormError(target) {
    target.textContent = "";
    target.hidden = true;
  }

  function openDialog(dialog) {
    if (!dialog.open) {
      dialog.showModal();
    }
  }

  function closeDialog(dialog) {
    if (dialog.open) {
      dialog.close();
    }
  }

  function closeAllDialogs() {
    closeDialog(elements.loginDialog);
    closeDialog(elements.objectDialog);
    closeDialog(elements.busyDialog);
    closeDialog(elements.reserveDialog);
    closeDialog(elements.categoryDialog);
    closeDialog(elements.detailDialog);
  }

  function applyAdminVisibility() {
    const visible = Boolean(state.user);
    for (const node of document.querySelectorAll("[data-admin-only]")) {
      node.hidden = !visible;
    }
  }

  function renderAuth() {
    const loggedIn = Boolean(state.user);
    elements.loginButton.hidden = loggedIn;
    elements.userArea.hidden = !loggedIn;
    elements.userEmail.textContent = loggedIn ? state.user.email : "";
    applyAdminVisibility();
  }

  function hasFilters() {
    return Boolean(state.filters.q || state.filters.category || state.filters.tag);
  }

  function formatDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "";
    }
    return date.toLocaleDateString("it-IT", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  }

  function toDateTimeLocal(date) {
    const pad = (value) => String(value).padStart(2, "0");
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  }

  function parseTags(value) {
    const tags = [];
    const seen = new Set();
    for (const part of value.split(",")) {
      const tag = part.trim().toLowerCase();
      if (tag && !seen.has(tag)) {
        seen.add(tag);
        tags.push(tag);
      }
    }
    return tags;
  }

  // Categories cycle through the palette in list order so neighbours differ;
  // other names fall back to a stable hash.
  function toneFor(name) {
    const index = state.categories.findIndex((category) => category.name === name);
    if (index >= 0) {
      return String(index % 5);
    }
    let hash = 0;
    for (const char of String(name)) {
      hash = (hash * 31 + char.codePointAt(0)) >>> 0;
    }
    return String(hash % 5);
  }

  function initialOf(name) {
    const first = String(name).trim()[0];
    return first ? first.toUpperCase() : "?";
  }

  function plural(count, one, many) {
    return `${count} ${count === 1 ? one : many}`;
  }

  function availabilityBadge(object) {
    if (object.busy) {
      const label = object.busy_until
        ? `In prestito fino al ${formatDate(object.busy_until)}`
        : "In prestito";
      return el("span", "busy-badge", label);
    }
    if (object.reserved_from) {
      return el("span", "reserved-badge", `Prenotato dal ${formatDate(object.reserved_from)}`);
    }
    return el("span", "available-badge", "Disponibile");
  }

  function hidePhotoImage() {
    if (photoObjectURL) {
      URL.revokeObjectURL(photoObjectURL);
      photoObjectURL = "";
    }
    elements.objectPhotoPreview.removeAttribute("src");
    elements.objectPhotoPreview.hidden = true;
  }

  function clearPhotoPreview() {
    hidePhotoImage();
    elements.objectPhotoPreviewWrap.hidden = true;
  }

  function showPhotoPreview(source, isObjectURL = false) {
    if (photoObjectURL) {
      URL.revokeObjectURL(photoObjectURL);
      photoObjectURL = "";
    }
    if (!source) {
      clearPhotoPreview();
      return;
    }
    elements.objectPhotoPreview.src = source;
    elements.objectPhotoPreview.hidden = false;
    elements.objectPhotoPreviewWrap.hidden = false;
    if (isObjectURL) {
      photoObjectURL = source;
    }
  }

  function currentObjectImagePath() {
    const object = state.objects.find((item) => item.id === state.editingObjectId);
    return object ? object.image_path : "";
  }

  function revokePendingImageURLs() {
    for (const url of pendingImageURLs) {
      URL.revokeObjectURL(url);
    }
    pendingImageURLs = [];
  }

  // Client-side compression: cap the longest edge at 1600px (never upscale)
  // and re-encode as JPEG on white so PNG alpha flattens. Falls back to the
  // original file when decoding fails or compression does not help.
  async function compressImage(file) {
    try {
      const bitmap = await createImageBitmap(file);
      const longest = Math.max(bitmap.width, bitmap.height);
      const scaled = longest > 1600;
      const ratio = scaled ? 1600 / longest : 1;
      const width = Math.max(1, Math.round(bitmap.width * ratio));
      const height = Math.max(1, Math.round(bitmap.height * ratio));
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext("2d");
      ctx.fillStyle = "#ffffff";
      ctx.fillRect(0, 0, width, height);
      ctx.drawImage(bitmap, 0, 0, width, height);
      bitmap.close();
      const blob = await new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", 0.8));
      if (!blob || (!scaled && blob.size >= file.size)) {
        return file;
      }
      const name = String(file.name || "immagine").replace(/\.[^.]*$/, "");
      return new File([blob], name + ".jpg", { type: "image/jpeg" });
    } catch {
      return file;
    }
  }

  function clearPendingUploads() {
    revokePendingImageURLs();
    state.pendingUploads = [];
    elements.objectImages.value = "";
    renderImagesStrip();
  }

  function renderImagesStrip() {
    elements.objectImagesStrip.replaceChildren();
    if (state.editingObjectImages.length === 0 && state.pendingUploads.length === 0) {
      elements.objectImagesStrip.hidden = true;
      return;
    }
    elements.objectImagesStrip.hidden = false;

    state.editingObjectImages.forEach((image, index) => {
      const tile = el("div", "image-tile");
      const thumb = el("img", "image-thumb");
      thumb.src = image.url;
      thumb.alt = "Immagine dell'oggetto";
      thumb.loading = "lazy";
      thumb.decoding = "async";
      thumb.addEventListener("error", () => thumb.remove());
      tile.append(thumb);

      const actions = el("div", "image-tile-actions");
      const removeButton = el("button", "button button-danger button-small", "✕");
      removeButton.type = "button";
      removeButton.dataset.action = "remove-image";
      removeButton.dataset.imageId = image.id;
      removeButton.setAttribute("aria-label", "Rimuovi immagine");
      const leftButton = el("button", "button button-ghost button-small", "←");
      leftButton.type = "button";
      leftButton.dataset.action = "move-image";
      leftButton.dataset.imageId = image.id;
      leftButton.dataset.delta = "-1";
      leftButton.setAttribute("aria-label", "Sposta prima");
      leftButton.disabled = index === 0;
      const rightButton = el("button", "button button-ghost button-small", "→");
      rightButton.type = "button";
      rightButton.dataset.action = "move-image";
      rightButton.dataset.imageId = image.id;
      rightButton.dataset.delta = "1";
      rightButton.setAttribute("aria-label", "Sposta dopo");
      rightButton.disabled = index === state.editingObjectImages.length - 1;
      actions.append(removeButton, leftButton, rightButton);
      tile.append(actions);
      elements.objectImagesStrip.append(tile);
    });

    pendingImageURLs.forEach((url, index) => {
      const tile = el("div", "image-tile");
      const thumb = el("img", "image-thumb");
      thumb.src = url;
      thumb.alt = "Anteprima caricamento";
      tile.append(thumb);

      const actions = el("div", "image-tile-actions");
      const removeButton = el("button", "button button-danger button-small", "✕");
      removeButton.type = "button";
      removeButton.dataset.action = "remove-pending";
      removeButton.dataset.index = String(index);
      removeButton.setAttribute("aria-label", "Rimuovi caricamento");
      actions.append(removeButton);
      tile.append(actions);
      elements.objectImagesStrip.append(tile);
    });
  }

  function applyObjectImages(object) {
    if (!object) {
      return;
    }
    state.editingObjectImages = object.images || [];
    const cached = state.objects.find((item) => item.id === object.id);
    if (cached) {
      cached.image_path = object.image_path;
      cached.images = state.editingObjectImages;
    }
    if (!photoObjectURL) {
      showPhotoPreview(object.image_path);
    }
    renderImagesStrip();
  }

  async function removeServerImage(imageId) {
    if (!state.editingObjectId) {
      return;
    }
    try {
      const data = await api(
        `/api/objects/${encodeURIComponent(state.editingObjectId)}/images/${encodeURIComponent(imageId)}`,
        { method: "DELETE" },
      );
      applyObjectImages(data.object);
    } catch (error) {
      handleActionError(error);
    }
  }

  function removePendingUpload(index) {
    if (index < 0 || index >= state.pendingUploads.length) {
      return;
    }
    URL.revokeObjectURL(pendingImageURLs[index]);
    pendingImageURLs.splice(index, 1);
    state.pendingUploads.splice(index, 1);
    renderImagesStrip();
  }

  async function moveServerImage(imageId, delta) {
    if (!state.editingObjectId) {
      return;
    }
    const ordered = state.editingObjectImages.slice();
    const index = ordered.findIndex((image) => image.id === imageId);
    const target = index + delta;
    if (index < 0 || target < 0 || target >= ordered.length) {
      return;
    }
    const [moved] = ordered.splice(index, 1);
    ordered.splice(target, 0, moved);
    try {
      const data = await api(`/api/objects/${encodeURIComponent(state.editingObjectId)}/images/order`, {
        method: "PATCH",
        body: { image_ids: ordered.map((image) => image.id) },
      });
      applyObjectImages(data.object);
    } catch (error) {
      handleActionError(error);
    }
  }

  function populateCategorySelects() {
    const selectedFilter = state.filters.category;
    elements.categoryFilter.replaceChildren(option("", "Tutte le categorie"));
    for (const category of state.categories) {
      elements.categoryFilter.append(option(category.name, category.name));
    }
    const filterExists = state.categories.some((category) => category.name === selectedFilter);
    elements.categoryFilter.value = filterExists ? selectedFilter : "";
    if (!filterExists) {
      state.filters.category = "";
    }

    const selectedObjectCategory = elements.objectCategory.value;
    elements.objectCategory.replaceChildren(option("", "Nessuna categoria"));
    for (const category of state.categories) {
      elements.objectCategory.append(option(category.id, category.name));
    }
    const categoryExists = state.categories.some(
      (category) => category.id === selectedObjectCategory,
    );
    elements.objectCategory.value = categoryExists ? selectedObjectCategory : "";
  }

  function renderCategories() {
    elements.categoryList.replaceChildren();
    if (state.categories.length === 0) {
      elements.categoryList.append(
        el("p", "muted", "Nessuna categoria. Aggiungine una."),
      );
      applyAdminVisibility();
      return;
    }

    for (const category of state.categories) {
      const chip = el("div", "category-chip");
      const filterButton = el("button", "category-filter", category.name);
      filterButton.type = "button";
      filterButton.dataset.action = "filter-category";
      filterButton.dataset.name = category.name;
      if (state.filters.category === category.name) {
        filterButton.classList.add("is-active");
      }
      chip.append(filterButton);

      const actions = el("span", "category-actions");
      actions.dataset.adminOnly = "";
      const editButton = el("button", "button button-ghost button-small", "Rinomina");
      editButton.type = "button";
      editButton.dataset.action = "edit-category";
      editButton.dataset.id = category.id;
      editButton.setAttribute("aria-label", `Rinomina ${category.name}`);
      const deleteButton = el("button", "button button-danger button-small", "Elimina");
      deleteButton.type = "button";
      deleteButton.dataset.action = "delete-category";
      deleteButton.dataset.id = category.id;
      deleteButton.setAttribute("aria-label", `Elimina ${category.name}`);
      actions.append(editButton, deleteButton);
      chip.append(actions);
      elements.categoryList.append(chip);
    }
    applyAdminVisibility();
  }

  function renderObjects() {
    elements.objectGrid.replaceChildren();
    const empty = state.objects.length === 0;
    elements.emptyState.hidden = !empty;
    if (empty) {
      elements.emptyMessage.textContent = hasFilters()
        ? "Prova ad azzerare i filtri o a cercare qualcos'altro."
        : state.user
          ? "Crea il primo oggetto per iniziare."
          : "Il catalogo è ancora vuoto.";
      applyAdminVisibility();
      return;
    }

    for (const object of state.objects) {
      const reserved = !object.busy && object.reserved_from;
      const card = el("article", reserved ? "object-card object-card-reserved" : "object-card");
      card.dataset.id = object.id;
      card.tabIndex = 0;
      card.setAttribute("role", "button");
      card.setAttribute("aria-label", `Dettagli di ${object.name}`);

      const media = el("div", "object-media");
      media.dataset.tone = toneFor(object.category ? object.category.name : object.name);
      media.append(el("span", "object-media-letter", initialOf(object.name)));
      if (object.image_path) {
        const photo = el("img", "object-photo");
        photo.src = object.image_path;
        photo.alt = `Foto di ${object.name}`;
        photo.loading = "lazy";
        photo.decoding = "async";
        photo.addEventListener("error", () => photo.remove());
        media.append(photo);
      }
      const badge = availabilityBadge(object);
      badge.classList.add("availability");
      media.append(badge);
      card.append(media);

      const body = el("div", "object-body");
      if (object.category) {
        body.append(el("span", "card-category", object.category.name));
      }
      body.append(el("h3", "", object.name));
      body.append(
        el(
          "p",
          object.description ? "card-description" : "card-description is-empty",
          object.description || "Nessuna descrizione.",
        ),
      );

      if (object.tags.length > 0) {
        const tags = el("ul", "tag-list");
        for (const tag of object.tags) {
          tags.append(el("li", "tag", tag));
        }
        body.append(tags);
      }

      const footer = el("div", "card-footer");
      const date = el("time", "card-date", `Aggiunto il ${formatDate(object.created_at)}`);
      date.dateTime = object.created_at;
      footer.append(date);

      const actions = el("div", "card-actions");
      actions.dataset.adminOnly = "";
      const editButton = el("button", "button button-ghost button-small", "Modifica");
      editButton.type = "button";
      editButton.dataset.action = "edit-object";
      editButton.dataset.id = object.id;
      editButton.setAttribute("aria-label", `Modifica ${object.name}`);
      const taken = object.busy;
      const busyButton = el(
        "button",
        "button button-ghost button-small",
        taken ? "Restituito" : "In prestito",
      );
      busyButton.type = "button";
      busyButton.dataset.action = taken ? "release-object" : "busy-object";
      busyButton.dataset.id = object.id;
      busyButton.setAttribute(
        "aria-label",
        taken ? `Segna ${object.name} come restituito` : `Segna ${object.name} in prestito`,
      );
      const reserveButton = el(
        "button",
        "button button-ghost button-small",
        object.reserved_from ? "Cambia prenotazione" : "Prenota",
      );
      reserveButton.type = "button";
      reserveButton.dataset.action = "reserve-object";
      reserveButton.dataset.id = object.id;
      reserveButton.setAttribute("aria-label", `Prenota ${object.name}`);
      const deleteButton = el("button", "button button-danger button-small", "Elimina");
      deleteButton.type = "button";
      deleteButton.dataset.action = "delete-object";
      deleteButton.dataset.id = object.id;
      deleteButton.setAttribute("aria-label", `Elimina ${object.name}`);
      actions.append(editButton, busyButton, reserveButton, deleteButton);
      footer.append(actions);

      body.append(footer);
      card.append(body);
      elements.objectGrid.append(card);
    }
    applyAdminVisibility();
  }

  function renderPagination() {
    if (state.total === 0) {
      elements.pagination.hidden = true;
      return;
    }
    const pages = Math.max(1, Math.ceil(state.total / state.limit));
    const page = Math.floor(state.offset / state.limit) + 1;
    elements.pageInfo.textContent = `Pagina ${page} di ${pages}`;
    elements.prevPage.disabled = state.offset === 0;
    elements.nextPage.disabled = state.offset + state.limit >= state.total;
    elements.pagination.hidden = pages < 2;
  }

  function renderResultsHeading() {
    const { q, category, tag } = state.filters;
    let title = "Ultimi arrivi";
    if (category && !q && !tag) {
      title = category;
    } else if (hasFilters()) {
      title = "Risultati";
    }
    elements.resultsTitle.textContent = title;
    elements.resultsCount.textContent = plural(state.total, "oggetto", "oggetti");
  }

  function filterChip(label, key) {
    const chip = el("button", "filter-chip");
    chip.type = "button";
    chip.dataset.clear = key;
    chip.setAttribute("aria-label", `Rimuovi filtro ${label}`);
    chip.append(el("span", "", label), el("span", "filter-chip-x", "✕"));
    return chip;
  }

  function renderActiveFilters() {
    const { q, category, tag } = state.filters;
    elements.activeFilters.replaceChildren();
    elements.activeFilters.hidden = !hasFilters();
    if (!hasFilters()) {
      return;
    }
    elements.activeFilters.append(el("span", "active-filters-label", "Filtri:"));
    if (category) {
      elements.activeFilters.append(filterChip(`Categoria: ${category}`, "category"));
    }
    if (q) {
      elements.activeFilters.append(filterChip(`“${q}”`, "q"));
    }
    if (tag) {
      elements.activeFilters.append(filterChip(`#${tag}`, "tag"));
    }
  }

  // Category cards are the catalog's landing view; they step aside once a filter is set.
  function renderCategoryCards() {
    elements.categoryCards.replaceChildren();
    elements.categoryCardsSection.hidden = hasFilters() || state.categories.length === 0;
    for (const category of state.categories) {
      const stats = state.categoryStats.get(category.name);
      const card = el("button", "category-card");
      card.type = "button";
      card.dataset.name = category.name;
      card.dataset.tone = toneFor(category.name);
      if (stats && stats.cover) {
        card.classList.add("has-image");
        const image = el("img", "category-card-image");
        image.src = stats.cover;
        image.alt = "";
        image.loading = "lazy";
        image.decoding = "async";
        image.addEventListener("error", () => {
          image.remove();
          card.classList.remove("has-image");
        });
        card.append(image);
      } else {
        card.append(el("span", "category-card-letter", initialOf(category.name)));
      }
      const body = el("span", "category-card-body");
      body.append(el("span", "category-card-name", category.name));
      body.append(
        el("span", "category-card-count", stats ? plural(stats.total, "oggetto", "oggetti") : " "),
      );
      card.append(body);
      elements.categoryCards.append(card);
    }
  }

  // One small query per category yields its object count and its newest cover.
  async function loadCategoryStats() {
    const requestId = ++state.statsRequestId;
    const categories = state.categories.slice();
    const results = await Promise.all(
      categories.map(async (category) => {
        const params = new URLSearchParams({ category: category.name, limit: "12" });
        try {
          const data = await api(`/api/objects?${params.toString()}`);
          const withCover = data.objects.find((object) => object.image_path);
          return [category.name, { total: data.total, cover: withCover ? withCover.image_path : "" }];
        } catch {
          return null;
        }
      }),
    );
    if (requestId !== state.statsRequestId) {
      return;
    }
    state.categoryStats = new Map(results.filter(Boolean));
    renderCategoryCards();
  }

  function applyCategoryFilter(name) {
    state.filters.category = name;
    state.offset = 0;
    elements.categoryFilter.value = name;
    renderCategories();
    loadObjects();
    elements.catalogSection.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  function setLoading(loading) {
    elements.loading.hidden = !loading;
    elements.catalogSection.setAttribute("aria-busy", loading ? "true" : "false");
  }

  async function loadSession() {
    try {
      const data = await api("/api/auth/me");
      state.user = data.user;
    } catch (error) {
      state.user = null;
      if (error.status !== 401) {
        showStatus(error.message, "error");
      }
    }
    renderAuth();
  }

  async function loadCategories() {
    try {
      const data = await api("/api/categories");
      state.categories = data.categories;
      elements.statCategories.textContent = String(state.categories.length);
      populateCategorySelects();
      renderCategories();
      renderCategoryCards();
      loadCategoryStats();
    } catch (error) {
      showStatus(error.message, "error");
    }
  }

  async function loadObjects() {
    const requestId = ++state.requestId;
    setLoading(true);
    const params = new URLSearchParams({
      limit: String(state.limit),
      offset: String(state.offset),
    });
    if (state.filters.q) {
      params.set("q", state.filters.q);
    }
    if (state.filters.category) {
      params.set("category", state.filters.category);
    }
    if (state.filters.tag) {
      params.set("tag", state.filters.tag);
    }

    try {
      const data = await api(`/api/objects?${params.toString()}`);
      if (requestId !== state.requestId) {
        return;
      }
      state.objects = data.objects;
      state.total = data.total;
      renderObjects();
      renderPagination();
      if (!hasFilters()) {
        elements.statObjects.textContent = String(data.total);
        loadCategoryStats();
      }
    } catch (error) {
      if (requestId !== state.requestId) {
        return;
      }
      state.objects = [];
      state.total = 0;
      renderObjects();
      renderPagination();
      showStatus(error.message, "error");
    } finally {
      if (requestId === state.requestId) {
        setLoading(false);
        renderResultsHeading();
        renderActiveFilters();
        renderCategoryCards();
        loadReservations();
      }
    }
  }

  async function loadReservations() {
    if (!elements.reservationsList) {
      return;
    }
    if (!state.user) {
      elements.reservationsList.replaceChildren();
      return;
    }
    let reserved = [];
    try {
      const data = await api("/api/objects?limit=100");
      reserved = data.objects.filter((object) => object.reservation);
    } catch (error) {
      elements.reservationsList.replaceChildren(
        el("p", "muted", `Impossibile caricare le prenotazioni: ${error.message}`),
      );
      return;
    }
    if (reserved.length === 0) {
      elements.reservationsList.replaceChildren(
        el("p", "muted", "Nessuna prenotazione attiva."),
      );
      return;
    }
    reserved.sort((a, b) => new Date(a.reservation.from) - new Date(b.reservation.from));
    const rows = reserved.map((object) => {
      const row = el("div", "reservation-row");
      const info = el("div", "reservation-info");
      info.append(el("span", "reservation-name", object.name));
      const period = el(
        "span",
        "reservation-window",
        `${formatDate(object.reservation.from)} → ${object.reservation.until ? formatDate(object.reservation.until) : "senza scadenza"}`,
      );
      info.append(period);
      row.append(info);
      const status = el("span", "reservation-status", object.reservation.status);
      row.append(status);
      const button = el("button", "button button-ghost button-small", "Modifica");
      button.type = "button";
      button.dataset.action = "reserve-object";
      button.dataset.id = object.id;
      button.setAttribute("aria-label", `Modifica la prenotazione di ${object.name}`);
      row.append(button);
      return row;
    });
    elements.reservationsList.replaceChildren(...rows);
  }

  function openLogin(message = "") {
    hideFormError(elements.loginError);
    if (message) {
      showFormError(elements.loginError, message);
    }
    openDialog(elements.loginDialog);
    elements.loginEmail.focus();
  }

  function handleFormError(error, target, dialog) {
    if (error.status === 401) {
      closeDialog(dialog);
      openLogin("Accedi per continuare.");
      return;
    }
    showFormError(target, error.message);
  }

  function handleActionError(error) {
    if (error.status === 401) {
      openLogin("Accedi per continuare.");
      return;
    }
    showStatus(error.message, "error");
  }

  function openNewObject() {
    state.editingObjectId = null;
    elements.objectForm.reset();
    elements.objectId.value = "";
    elements.objectDialogTitle.textContent = "Nuovo oggetto";
    elements.objectSave.textContent = "Crea oggetto";
    elements.objectPhoto.value = "";
    state.pendingPhoto = null;
    elements.objectRemovePhoto.checked = false;
    clearPhotoPreview();
    state.editingObjectImages = [];
    clearPendingUploads();
    populateCategorySelects();
    hideFormError(elements.objectError);
    openDialog(elements.objectDialog);
    elements.objectName.focus();
  }

  function openEditObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    state.editingObjectId = id;
    elements.objectForm.reset();
    elements.objectId.value = object.id;
    elements.objectName.value = object.name;
    elements.objectDescription.value = object.description;
    elements.objectTags.value = object.tags.join(", ");
    elements.objectDialogTitle.textContent = "Modifica oggetto";
    elements.objectSave.textContent = "Salva modifiche";
    elements.objectPhoto.value = "";
    state.pendingPhoto = null;
    elements.objectRemovePhoto.checked = false;
    showPhotoPreview(object.image_path);
    state.editingObjectImages = (object.images || []).slice();
    clearPendingUploads();
    populateCategorySelects();
    elements.objectCategory.value = object.category ? object.category.id : "";
    hideFormError(elements.objectError);
    openDialog(elements.objectDialog);
    elements.objectName.focus();
  }

  function openBusyObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    state.editingBusyId = id;
    elements.busyForm.reset();
    elements.busyObjectId.value = object.id;
    elements.busyDialogTitle.textContent = `Segna "${object.name}" in prestito`;
    const minimum = new Date(Date.now() + 60 * 1000);
    const suggested = new Date(Date.now() + 24 * 60 * 60 * 1000);
    elements.busyUntil.min = toDateTimeLocal(minimum);
    elements.busyUntil.value = toDateTimeLocal(suggested);
    hideFormError(elements.busyError);
    openDialog(elements.busyDialog);
    elements.busyUntil.focus();
  }

  function openReserveObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    elements.reserveForm.reset();
    elements.reserveObjectId.value = object.id;
    elements.reserveDialogTitle.textContent = `Prenota "${object.name}"`;
    const minimum = new Date(Date.now() + 60 * 1000);
    const suggested = new Date(Date.now() + 24 * 60 * 60 * 1000);
    elements.reserveFrom.min = toDateTimeLocal(minimum);
    elements.reserveFrom.value = toDateTimeLocal(suggested);
    elements.reserveUntil.min = toDateTimeLocal(minimum);
    elements.reserveUntil.value = "";
    hideFormError(elements.reserveError);
    openDialog(elements.reserveDialog);
    elements.reserveFrom.focus();
  }

  async function releaseObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    if (!window.confirm(`Segnare "${object.name}" come restituito?`)) {
      return;
    }
    try {
      await api(`/api/objects/${encodeURIComponent(id)}/busy`, {
        method: "PATCH",
        body: { busy: false },
      });
      showStatus("Oggetto restituito.", "success");
      await loadObjects();
    } catch (error) {
      handleActionError(error);
    }
  }

  function openNewCategory() {
    state.editingCategoryId = null;
    elements.categoryForm.reset();
    elements.categoryId.value = "";
    elements.categoryDialogTitle.textContent = "Nuova categoria";
    elements.categorySave.textContent = "Crea categoria";
    hideFormError(elements.categoryError);
    openDialog(elements.categoryDialog);
    elements.categoryName.focus();
  }

  function openEditCategory(id) {
    const category = state.categories.find((item) => item.id === id);
    if (!category) {
      return;
    }
    state.editingCategoryId = id;
    elements.categoryForm.reset();
    elements.categoryId.value = category.id;
    elements.categoryName.value = category.name;
    elements.categoryDialogTitle.textContent = "Rinomina categoria";
    elements.categorySave.textContent = "Salva modifiche";
    hideFormError(elements.categoryError);
    openDialog(elements.categoryDialog);
    elements.categoryName.focus();
  }

  async function deleteObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    if (!window.confirm(`Eliminare "${object.name}"?`)) {
      return;
    }
    try {
      await api(`/api/objects/${encodeURIComponent(id)}`, { method: "DELETE" });
      showStatus("Oggetto eliminato.", "success");
      if (state.objects.length === 1 && state.offset > 0) {
        state.offset = Math.max(0, state.offset - state.limit);
      }
      await loadObjects();
    } catch (error) {
      handleActionError(error);
    }
  }

  async function deleteCategory(id) {
    const category = state.categories.find((item) => item.id === id);
    if (!category) {
      return;
    }
    if (
      !window.confirm(
        `Eliminare "${category.name}"? I suoi oggetti resteranno senza categoria.`,
      )
    ) {
      return;
    }
    try {
      await api(`/api/categories/${encodeURIComponent(id)}`, { method: "DELETE" });
      showStatus("Categoria eliminata. I suoi oggetti sono ora senza categoria.", "success");
      await loadCategories();
      await loadObjects();
    } catch (error) {
      handleActionError(error);
    }
  }

  const detailHash = (id) => `#object/${encodeURIComponent(id)}`;

  function detailImageURLs(object) {
    const images = (object.images || []).map((image) => image.url).filter(Boolean);
    if (images.length === 0 && object.image_path) {
      return [object.image_path];
    }
    return images;
  }

  function renderDetailGallery() {
    const urls = state.detailImages;
    const count = urls.length;
    if (count === 0) {
      elements.detailStage.hidden = true;
      elements.detailThumbs.hidden = true;
      elements.detailThumbs.replaceChildren();
      elements.detailCaption.textContent = "";
      return;
    }
    const index = Math.min(Math.max(state.detailIndex, 0), count - 1);
    state.detailIndex = index;
    elements.detailStage.hidden = false;
    elements.detailMainImage.src = urls[index];
    elements.detailMainImage.alt = `${state.detailName || "Oggetto"}: immagine ${index + 1} di ${count}`;
    elements.detailPrev.disabled = count < 2;
    elements.detailNext.disabled = count < 2;
    elements.detailCaption.textContent = count > 1 ? `Immagine ${index + 1} di ${count}` : "";

    elements.detailThumbs.hidden = count < 2;
    elements.detailThumbs.replaceChildren();
    if (count < 2) {
      return;
    }
    urls.forEach((url, position) => {
      const thumb = el("button", "detail-thumb");
      thumb.type = "button";
      thumb.dataset.index = String(position);
      const image = el("img", "detail-thumb-image");
      image.src = url;
      image.alt = "";
      image.loading = "lazy";
      image.decoding = "async";
      thumb.append(image);
      if (position === index) {
        thumb.classList.add("is-active");
        thumb.setAttribute("aria-current", "true");
      }
      thumb.setAttribute("aria-label", `Mostra immagine ${position + 1} di ${count}`);
      elements.detailThumbs.append(thumb);
    });
  }

  function showDetailObject(object) {
    state.detailName = object.name;
    state.detailImages = detailImageURLs(object);
    state.detailIndex = 0;

    elements.detailTitle.textContent = object.name;
    elements.detailBadges.replaceChildren();
    if (object.category) {
      elements.detailBadges.append(el("span", "category-badge", object.category.name));
    }
    if (object.busy) {
      const label = object.busy_until
        ? `Unavailable until ${formatDate(object.busy_until)}`
        : "Unavailable";
      elements.detailBadges.append(el("span", "busy-badge", label));
    } else if (object.reserved_from) {
      elements.detailBadges.append(
        el("span", "reserved-badge", `Reserved from ${formatDate(object.reserved_from)}`),
      );
    }

    renderDetailGallery();

    elements.detailDescription.textContent = object.description
      ? object.description
      : "No description.";
    elements.detailDescription.classList.toggle("is-empty", !object.description);

    elements.detailTags.replaceChildren();
    for (const tag of object.tags || []) {
      elements.detailTags.append(el("li", "tag", tag));
    }
    elements.detailTags.hidden = (object.tags || []).length === 0;

    const dates = [`Aggiunto il ${formatDate(object.created_at)}`];
    if (object.updated_at && object.updated_at !== object.created_at) {
      dates.push(`Aggiornato il ${formatDate(object.updated_at)}`);
    }
    elements.detailDates.textContent = dates.join(" · ");

    elements.detailLoading.hidden = true;
    elements.detailContent.hidden = false;
    applyAdminVisibility();
    openDialog(elements.detailDialog);
  }

  async function showDetail(id) {
    const requestId = ++state.detailRequestId;
    state.detailObjectId = id;
    state.detailName = "";
    state.detailImages = [];
    state.detailIndex = 0;
    elements.detailContent.hidden = true;
    elements.detailLoading.hidden = false;
    openDialog(elements.detailDialog);
    try {
      const data = await api(`/api/objects/${encodeURIComponent(id)}`);
      if (requestId !== state.detailRequestId || state.detailObjectId !== id) {
        return;
      }
      showDetailObject(data.object);
    } catch (error) {
      if (requestId !== state.detailRequestId) {
        return;
      }
      state.detailObjectId = null;
      elements.detailLoading.hidden = true;
      closeDialog(elements.detailDialog);
      handleActionError(error);
    }
  }

  function currentDetailIdFromHash() {
    const match = /^#object\/(.+)$/.exec(location.hash);
    if (!match) {
      return "";
    }
    try {
      return decodeURIComponent(match[1]);
    } catch {
      return "";
    }
  }

  function showView(name) {
    if (state.view === name) {
      return;
    }
    const firstRender = state.view === "";
    state.view = name;
    for (const view of elements.views) {
      view.hidden = view.dataset.view !== name;
    }
    for (const link of elements.viewLinks) {
      if (link.dataset.viewLink === name) {
        link.setAttribute("aria-current", "page");
      } else {
        link.removeAttribute("aria-current");
      }
    }
    document.title = name === "catalog"
      ? "Catalogo · Biblioteca degli Oggetti"
      : "Biblioteca degli Oggetti";
    if (!firstRender) {
      window.scrollTo({ top: 0 });
    }
  }

  function handleRoute() {
    const id = currentDetailIdFromHash();
    showView(id || location.hash === "#catalogo" ? "catalog" : "home");
    if (id) {
      if (id !== state.detailObjectId) {
        showDetail(id);
      } else if (!elements.detailDialog.open) {
        showDetail(id);
      }
      return;
    }
    if (elements.detailDialog.open) {
      closeDialog(elements.detailDialog);
    }
  }

  function openDetail(id) {
    const target = detailHash(id);
    if (location.hash === target) {
      handleRoute();
      return;
    }
    location.hash = target;
  }

  function clearDetailHash() {
    if (location.hash.startsWith("#object/")) {
      history.pushState(null, "", `${location.pathname}${location.search}#catalogo`);
    }
  }

  function bindEvents() {
    elements.loginButton.addEventListener("click", () => openLogin());

    elements.loginForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.loginError);
      const submit = elements.loginForm.querySelector('button[type="submit"]');
      submit.disabled = true;
      try {
        const data = await api("/api/auth/login", {
          method: "POST",
          body: {
            email: elements.loginEmail.value.trim(),
            password: elements.loginPassword.value,
          },
        });
        state.user = data.user;
        elements.loginForm.reset();
        closeDialog(elements.loginDialog);
        renderAuth();
        renderCategories();
        renderObjects();
        showStatus(`Accesso effettuato come ${state.user.email}.`, "success");
      } catch (error) {
        showFormError(elements.loginError, error.message);
      } finally {
        submit.disabled = false;
      }
    });

    elements.logoutButton.addEventListener("click", async () => {
      try {
        await api("/api/auth/logout", { method: "POST" });
      } catch (error) {
        if (error.status !== 401) {
          showStatus(error.message, "error");
        }
      }
      state.user = null;
      renderAuth();
      renderCategories();
      renderObjects();
      showStatus("Disconnesso.", "success");
    });

    elements.newObjectButton.addEventListener("click", openNewObject);
    elements.newCategoryButton.addEventListener("click", openNewCategory);

    elements.objectForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.objectError);
      const name = elements.objectName.value.trim();
      if (!name) {
        showFormError(elements.objectError, "Il nome è obbligatorio.");
        elements.objectName.focus();
        return;
      }
      const formData = new FormData();
      formData.append("name", name);
      formData.append("description", elements.objectDescription.value.trim());
      formData.append("category_id", elements.objectCategory.value);
      formData.append("tags", parseTags(elements.objectTags.value).join(","));
      const photo = state.pendingPhoto;
      if (photo) {
        formData.append("photo", photo);
      } else if (state.editingObjectId && elements.objectRemovePhoto.checked) {
        formData.append("remove_photo", "true");
      }
      elements.objectSave.disabled = true;
      let objectId = state.editingObjectId;
      let saved = false;
      try {
        if (objectId) {
          await api(`/api/objects/${encodeURIComponent(objectId)}`, {
            method: "PATCH",
            body: formData,
          });
          showStatus("Oggetto aggiornato.", "success");
        } else {
          const data = await api("/api/objects", { method: "POST", body: formData });
          objectId = data.object.id;
          showStatus("Oggetto creato.", "success");
        }
        saved = true;
        if (state.pendingUploads.length > 0) {
          const imagesForm = new FormData();
          for (const file of state.pendingUploads) {
            imagesForm.append("images", file);
          }
          const data = await api(`/api/objects/${encodeURIComponent(objectId)}/images`, {
            method: "POST",
            body: imagesForm,
          });
          state.editingObjectImages = data.object.images || [];
          clearPendingUploads();
          showStatus("Immagini caricate.", "success");
        }
        closeDialog(elements.objectDialog);
        await loadObjects();
      } catch (error) {
        if (saved) {
          // The object is persisted; keep the dialog open so chosen files are not lost.
          if (objectId !== state.editingObjectId) {
            state.editingObjectId = objectId;
            elements.objectDialogTitle.textContent = "Modifica oggetto";
            elements.objectSave.textContent = "Salva modifiche";
          }
          showFormError(elements.objectError, error.message);
          await loadObjects();
        } else {
          handleFormError(error, elements.objectError, elements.objectDialog);
        }
      } finally {
        elements.objectSave.disabled = false;
      }
    });

    elements.busyForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.busyError);
      const value = elements.busyUntil.value;
      const until = new Date(value);
      if (!value || Number.isNaN(until.getTime()) || until.getTime() <= Date.now()) {
        showFormError(elements.busyError, "Scegli una data futura.");
        elements.busyUntil.focus();
        return;
      }
      elements.busySave.disabled = true;
      try {
        await api(`/api/objects/${encodeURIComponent(elements.busyObjectId.value)}/busy`, {
          method: "PATCH",
          body: { busy: true, busy_until: until.toISOString() },
        });
        showStatus("Oggetto segnato in prestito.", "success");
        closeDialog(elements.busyDialog);
        state.editingBusyId = null;
        await loadObjects();
      } catch (error) {
        handleFormError(error, elements.busyError, elements.busyDialog);
      } finally {
        elements.busySave.disabled = false;
      }
    });

    elements.reserveForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.reserveError);
      const fromValue = elements.reserveFrom.value;
      const from = new Date(fromValue);
      if (!fromValue || Number.isNaN(from.getTime()) || from.getTime() <= Date.now()) {
        showFormError(elements.reserveError, "Scegli un inizio prenotazione futuro.");
        elements.reserveFrom.focus();
        return;
      }
      const untilValue = elements.reserveUntil.value;
      const until = untilValue ? new Date(untilValue) : null;
      if (until && Number.isNaN(until.getTime())) {
        showFormError(elements.reserveError, "Scegli una fine prenotazione valida.");
        elements.reserveUntil.focus();
        return;
      }
      if (until && until.getTime() <= from.getTime()) {
        showFormError(elements.reserveError, "La fine deve essere successiva all'inizio.");
        elements.reserveUntil.focus();
        return;
      }
      elements.reserveSave.disabled = true;
      try {
        await api(`/api/objects/${encodeURIComponent(elements.reserveObjectId.value)}/reservation`, {
          method: "PATCH",
          body: { from: from.toISOString(), until: until ? until.toISOString() : null },
        });
        showStatus("Oggetto prenotato.", "success");
        closeDialog(elements.reserveDialog);
        await loadObjects();
      } catch (error) {
        handleFormError(error, elements.reserveError, elements.reserveDialog);
      } finally {
        elements.reserveSave.disabled = false;
      }
    });

    elements.categoryForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.categoryError);
      const name = elements.categoryName.value.trim();
      if (!name) {
        showFormError(elements.categoryError, "Il nome è obbligatorio.");
        elements.categoryName.focus();
        return;
      }
      elements.categorySave.disabled = true;
      try {
        if (state.editingCategoryId) {
          await api(`/api/categories/${encodeURIComponent(state.editingCategoryId)}`, {
            method: "PATCH",
            body: { name },
          });
          showStatus("Categoria aggiornata.", "success");
        } else {
          await api("/api/categories", { method: "POST", body: { name } });
          showStatus("Categoria creata.", "success");
        }
        closeDialog(elements.categoryDialog);
        await loadCategories();
        await loadObjects();
      } catch (error) {
        handleFormError(error, elements.categoryError, elements.categoryDialog);
      } finally {
        elements.categorySave.disabled = false;
      }
    });

    for (const button of document.querySelectorAll("[data-close-dialog]")) {
      button.addEventListener("click", () => {
        const dialog = document.getElementById(button.dataset.closeDialog);
        if (dialog) {
          closeDialog(dialog);
        }
      });
    }

    const debouncedSearch = debounce(() => {
      state.filters.q = elements.searchInput.value.trim();
      state.offset = 0;
      loadObjects();
    }, 300);

    const debouncedTag = debounce(() => {
      state.filters.tag = elements.tagFilter.value.trim();
      state.offset = 0;
      loadObjects();
    }, 300);

    elements.searchInput.addEventListener("input", debouncedSearch);
    elements.tagFilter.addEventListener("input", debouncedTag);

    elements.categoryFilter.addEventListener("change", () => {
      state.filters.category = elements.categoryFilter.value;
      state.offset = 0;
      renderCategories();
      loadObjects();
    });

    elements.clearFilters.addEventListener("click", () => {
      state.filters = { q: "", category: "", tag: "" };
      state.offset = 0;
      elements.searchInput.value = "";
      elements.categoryFilter.value = "";
      elements.tagFilter.value = "";
      renderCategories();
      loadObjects();
    });

    elements.prevPage.addEventListener("click", () => {
      if (state.offset === 0) {
        return;
      }
      state.offset = Math.max(0, state.offset - state.limit);
      elements.catalogSection.scrollIntoView({ behavior: "smooth", block: "start" });
      loadObjects();
    });

    elements.nextPage.addEventListener("click", () => {
      if (state.offset + state.limit >= state.total) {
        return;
      }
      state.offset += state.limit;
      elements.catalogSection.scrollIntoView({ behavior: "smooth", block: "start" });
      loadObjects();
    });

    elements.objectPhoto.addEventListener("change", async () => {
      hideFormError(elements.objectError);
      const file = elements.objectPhoto.files[0];
      if (!file) {
        state.pendingPhoto = null;
        elements.objectRemovePhoto.checked = false;
        showPhotoPreview(currentObjectImagePath());
        return;
      }
      const photo = await compressImage(file);
      if (photo.size > 5 * 1024 * 1024) {
        state.pendingPhoto = null;
        elements.objectPhoto.value = "";
        showFormError(elements.objectError, "La foto deve essere al massimo 5 MB.");
        showPhotoPreview(currentObjectImagePath());
        return;
      }
      state.pendingPhoto = photo;
      elements.objectRemovePhoto.checked = false;
      showPhotoPreview(URL.createObjectURL(photo), true);
    });

    elements.objectImages.addEventListener("change", async () => {
      hideFormError(elements.objectError);
      const files = Array.from(elements.objectImages.files);
      elements.objectImages.value = "";
      if (files.length === 0) {
        return;
      }
      const compressed = await Promise.all(files.map((file) => compressImage(file)));
      let rejectedSize = false;
      let rejectedCount = false;
      const room = 10 - state.editingObjectImages.length - state.pendingUploads.length;
      let kept = 0;
      for (const photo of compressed) {
        if (photo.size > 5 * 1024 * 1024) {
          rejectedSize = true;
          continue;
        }
        if (kept >= room) {
          rejectedCount = true;
          continue;
        }
        state.pendingUploads.push(photo);
        pendingImageURLs.push(URL.createObjectURL(photo));
        kept += 1;
      }
      if (rejectedSize && rejectedCount) {
        showFormError(elements.objectError, "Massimo 10 immagini per oggetto, ciascuna al massimo 5 MB.");
      } else if (rejectedSize) {
        showFormError(elements.objectError, "La foto deve essere al massimo 5 MB.");
      } else if (rejectedCount) {
        showFormError(elements.objectError, "Massimo 10 immagini per oggetto.");
      }
      renderImagesStrip();
    });

    elements.objectImagesStrip.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action]");
      if (!button) {
        return;
      }
      if (button.dataset.action === "remove-image") {
        removeServerImage(button.dataset.imageId);
      } else if (button.dataset.action === "remove-pending") {
        removePendingUpload(Number(button.dataset.index));
      } else if (button.dataset.action === "move-image") {
        moveServerImage(button.dataset.imageId, Number(button.dataset.delta));
      }
    });

    elements.objectRemovePhoto.addEventListener("change", () => {
      if (elements.objectRemovePhoto.checked) {
        elements.objectPhoto.value = "";
        state.pendingPhoto = null;
        if (currentObjectImagePath()) {
          hidePhotoImage();
          elements.objectPhotoPreviewWrap.hidden = false;
        } else {
          clearPhotoPreview();
        }
      } else {
        showPhotoPreview(currentObjectImagePath());
      }
    });

    elements.objectDialog.addEventListener("close", () => {
      clearPhotoPreview();
      clearPendingUploads();
    });

    elements.objectGrid.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action]");
      if (button) {
        if (button.dataset.action === "edit-object") {
          openEditObject(button.dataset.id);
        } else if (button.dataset.action === "busy-object") {
          openBusyObject(button.dataset.id);
        } else if (button.dataset.action === "reserve-object") {
          openReserveObject(button.dataset.id);
        } else if (button.dataset.action === "release-object") {
          releaseObject(button.dataset.id);
        } else if (button.dataset.action === "delete-object") {
          deleteObject(button.dataset.id);
        }
        return;
      }
      const card = event.target.closest(".object-card");
      if (card && card.dataset.id) {
        openDetail(card.dataset.id);
      }
    });

    elements.objectGrid.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") {
        return;
      }
      const card = event.target.closest(".object-card");
      if (!card || card !== event.target || !card.dataset.id) {
        return;
      }
      event.preventDefault();
      openDetail(card.dataset.id);
    });

    elements.reservationsList.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action='reserve-object']");
      if (button) {
        openReserveObject(button.dataset.id);
      }
    });

    elements.categoryList.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action]");
      if (!button) {
        return;
      }
      if (button.dataset.action === "filter-category") {
        applyCategoryFilter(button.dataset.name);
      } else if (button.dataset.action === "edit-category") {
        openEditCategory(button.dataset.id);
      } else if (button.dataset.action === "delete-category") {
        deleteCategory(button.dataset.id);
      }
    });

    elements.categoryCards.addEventListener("click", (event) => {
      const card = event.target.closest(".category-card");
      if (card) {
        applyCategoryFilter(card.dataset.name);
      }
    });

    elements.activeFilters.addEventListener("click", (event) => {
      const chip = event.target.closest("button[data-clear]");
      if (!chip) {
        return;
      }
      const key = chip.dataset.clear;
      state.filters[key] = "";
      state.offset = 0;
      if (key === "q") {
        elements.searchInput.value = "";
      } else if (key === "tag") {
        elements.tagFilter.value = "";
      } else {
        elements.categoryFilter.value = "";
      }
      renderCategories();
      loadObjects();
    });

    for (const button of document.querySelectorAll("[data-scroll-to]")) {
      button.addEventListener("click", () => {
        document.getElementById(button.dataset.scrollTo)?.scrollIntoView({ block: "start" });
      });
    }

    const stepDetailImage = (delta) => {
      const count = state.detailImages.length;
      if (count < 2) {
        return;
      }
      state.detailIndex = (state.detailIndex + delta + count) % count;
      renderDetailGallery();
    };

    elements.detailPrev.addEventListener("click", () => stepDetailImage(-1));
    elements.detailNext.addEventListener("click", () => stepDetailImage(1));

    elements.detailThumbs.addEventListener("click", (event) => {
      const thumb = event.target.closest("button.detail-thumb");
      if (!thumb) {
        return;
      }
      state.detailIndex = Number(thumb.dataset.index);
      renderDetailGallery();
    });

    elements.detailEdit.addEventListener("click", () => {
      if (!state.detailObjectId) {
        return;
      }
      const id = state.detailObjectId;
      closeDialog(elements.detailDialog);
      openEditObject(id);
    });

    elements.detailDialog.addEventListener("close", () => {
      state.detailObjectId = null;
      state.detailImages = [];
      state.detailIndex = 0;
      state.detailName = "";
      elements.detailContent.hidden = true;
      elements.detailLoading.hidden = true;
      clearDetailHash();
    });

    window.addEventListener("hashchange", handleRoute);
  }

  async function init() {
    bindEvents();
    // Pick the view before data loads so a #catalogo deep link does not flash the home page.
    showView(currentDetailIdFromHash() || location.hash === "#catalogo" ? "catalog" : "home");
    await loadSession();
    await Promise.all([loadCategories(), loadObjects()]);
    handleRoute();
  }

  init();
})();
