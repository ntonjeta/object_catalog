(() => {
  "use strict";

  const elements = {
    loginButton: document.getElementById("login-button"),
    userArea: document.getElementById("user-area"),
    userEmail: document.getElementById("user-email"),
    logoutButton: document.getElementById("logout-button"),
    importButton: document.getElementById("import-button"),
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
    objectTags: document.getElementById("object-tags"),
    objectError: document.getElementById("object-error"),
    objectSave: document.getElementById("object-save"),
busyDialog: document.getElementById("busy-dialog"),
    busyForm: document.getElementById("busy-form"),
    busyDialogTitle: document.getElementById("busy-dialog-title"),
    busyObjectId: document.getElementById("busy-object-id"),
    busyUntil: document.getElementById("busy-until"),
    busyFrom: document.getElementById("busy-from"),
    busyError: document.getElementById("busy-error"),
    busySave: document.getElementById("busy-save"),
    importDialog: document.getElementById("import-dialog"),
    importForm: document.getElementById("import-form"),
    importFile: document.getElementById("import-file"),
    importResult: document.getElementById("import-result"),
    importError: document.getElementById("import-error"),
    importSave: document.getElementById("import-save"),
    categoryDialog: document.getElementById("category-dialog"),
    categoryForm: document.getElementById("category-form"),
    categoryDialogTitle: document.getElementById("category-dialog-title"),
    categoryId: document.getElementById("category-id"),
    categoryName: document.getElementById("category-name"),
    categoryError: document.getElementById("category-error"),
    categorySave: document.getElementById("category-save"),
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
    editingBusyId: null,
    editingCategoryId: null,
  };

  let statusTimer;
  let photoObjectURL = "";

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
      const error = new Error("Network error. Is the server running?");
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
      const message = payload?.error?.message || `Request failed (${response.status}).`;
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
    closeDialog(elements.categoryDialog);
    closeDialog(elements.importDialog);
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
    return date.toLocaleDateString(undefined, {
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

  function populateCategorySelects() {
    const selectedFilter = state.filters.category;
    elements.categoryFilter.replaceChildren(option("", "All categories"));
    for (const category of state.categories) {
      elements.categoryFilter.append(option(category.name, category.name));
    }
    const filterExists = state.categories.some((category) => category.name === selectedFilter);
    elements.categoryFilter.value = filterExists ? selectedFilter : "";
    if (!filterExists) {
      state.filters.category = "";
    }

    const selectedObjectCategory = elements.objectCategory.value;
    elements.objectCategory.replaceChildren(option("", "No category"));
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
        el("p", "muted", "No categories yet. An administrator can add one."),
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
      const editButton = el("button", "button button-ghost button-small", "Rename");
      editButton.type = "button";
      editButton.dataset.action = "edit-category";
      editButton.dataset.id = category.id;
      editButton.setAttribute("aria-label", `Rename ${category.name}`);
      const deleteButton = el("button", "button button-danger button-small", "Delete");
      deleteButton.type = "button";
      deleteButton.dataset.action = "delete-category";
      deleteButton.dataset.id = category.id;
      deleteButton.setAttribute("aria-label", `Delete ${category.name}`);
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
        ? "Try clearing the filters."
        : state.user
          ? "Create the first object to get started."
          : "The catalog is empty. An administrator can add objects.";
      applyAdminVisibility();
      return;
    }

    for (const object of state.objects) {
      const card = el("article", "object-card");

      const top = el("div", "card-top");
      top.append(el("h3", "", object.name));
      if (object.category) {
        top.append(el("span", "category-badge", object.category.name));
      }
      if (object.busy) {
        const label = object.busy_until
          ? `Unavailable until ${formatDate(object.busy_until)}`
          : "Unavailable";
        top.append(el("span", "busy-badge", label));
      } else if (object.reserved_from) {
        top.append(el("span", "busy-badge", `Reserved from ${formatDate(object.reserved_from)}`));
      }

      const description = el(
        "p",
        object.description ? "card-description" : "card-description is-empty",
        object.description || "No description.",
      );

      card.append(top);
      if (object.image_path) {
        const photo = el("img", "object-photo");
        photo.src = object.image_path;
        photo.alt = `Photo of ${object.name}`;
        photo.loading = "lazy";
        photo.decoding = "async";
        photo.addEventListener("error", () => photo.remove());
        card.append(photo);
      }
      card.append(description);

      if (object.tags.length > 0) {
        const tags = el("ul", "tag-list");
        for (const tag of object.tags) {
          tags.append(el("li", "tag", tag));
        }
        card.append(tags);
      }

      const footer = el("div", "card-footer");
      const date = el("time", "card-date", `Added ${formatDate(object.created_at)}`);
      date.dateTime = object.created_at;
      footer.append(date);

      const actions = el("div", "card-actions");
      actions.dataset.adminOnly = "";
      const editButton = el("button", "button button-ghost button-small", "Edit");
      editButton.type = "button";
      editButton.dataset.action = "edit-object";
      editButton.dataset.id = object.id;
      editButton.setAttribute("aria-label", `Edit ${object.name}`);
      const taken = object.busy || object.reserved_from;
      const busyButton = el(
        "button",
        "button button-ghost button-small",
        taken ? "Release" : "Mark busy",
      );
      busyButton.type = "button";
      busyButton.dataset.action = taken ? "release-object" : "busy-object";
      busyButton.dataset.id = object.id;
      busyButton.setAttribute(
        "aria-label",
        taken ? `Release ${object.name}` : `Mark ${object.name} busy`,
      );
      const deleteButton = el("button", "button button-danger button-small", "Delete");
      deleteButton.type = "button";
      deleteButton.dataset.action = "delete-object";
      deleteButton.dataset.id = object.id;
      deleteButton.setAttribute("aria-label", `Delete ${object.name}`);
      actions.append(editButton, busyButton, deleteButton);
      footer.append(actions);

      card.append(footer);
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
    const noun = state.total === 1 ? "object" : "objects";
    elements.pageInfo.textContent = `Page ${page} of ${pages} - ${state.total} ${noun}`;
    elements.prevPage.disabled = state.offset === 0;
    elements.nextPage.disabled = state.offset + state.limit >= state.total;
    elements.pagination.hidden = false;
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
      populateCategorySelects();
      renderCategories();
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
      }
    }
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
      openLogin("Please sign in to continue.");
      return;
    }
    showFormError(target, error.message);
  }

  function handleActionError(error) {
    if (error.status === 401) {
      openLogin("Please sign in to continue.");
      return;
    }
    showStatus(error.message, "error");
  }

  function openNewObject() {
    state.editingObjectId = null;
    elements.objectForm.reset();
    elements.objectId.value = "";
    elements.objectDialogTitle.textContent = "New object";
    elements.objectSave.textContent = "Create object";
    elements.objectPhoto.value = "";
    elements.objectRemovePhoto.checked = false;
    clearPhotoPreview();
    populateCategorySelects();
    hideFormError(elements.objectError);
    openDialog(elements.objectDialog);
    elements.objectName.focus();
  }

  function openImport() {
    elements.importForm.reset();
    elements.importResult.replaceChildren();
    elements.importResult.hidden = true;
    hideFormError(elements.importError);
    openDialog(elements.importDialog);
    elements.importFile.focus();
  }

  function renderImportResult(result) {
    elements.importResult.replaceChildren();
    const noun = result.failed === 1 ? "failure" : "failures";
    elements.importResult.append(
      el(
        "p",
        "",
        `Created ${result.created}, updated ${result.updated}, failed ${result.failed} ${noun}.`,
      ),
    );
    if (result.failures.length > 0) {
      const list = el("ul", "import-failures");
      for (const failure of result.failures) {
        list.append(el("li", "", `Row ${failure.row}: ${failure.message}`));
      }
      elements.importResult.append(list);
    }
    elements.importResult.hidden = false;
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
    elements.objectDialogTitle.textContent = "Edit object";
    elements.objectSave.textContent = "Save changes";
    elements.objectPhoto.value = "";
    elements.objectRemovePhoto.checked = false;
    showPhotoPreview(object.image_path);
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
    elements.busyDialogTitle.textContent = `Mark "${object.name}" busy`;
    const minimum = new Date(Date.now() + 60 * 1000);
    const suggested = new Date(Date.now() + 24 * 60 * 60 * 1000);
    elements.busyUntil.min = toDateTimeLocal(minimum);
    elements.busyUntil.value = toDateTimeLocal(suggested);
    elements.busyFrom.min = toDateTimeLocal(minimum);
    elements.busyFrom.value = "";
    hideFormError(elements.busyError);
    openDialog(elements.busyDialog);
    elements.busyUntil.focus();
  }

  async function releaseObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    if (!window.confirm(`Release "${object.name}"?`)) {
      return;
    }
    try {
      await api(`/api/objects/${encodeURIComponent(id)}/busy`, {
        method: "PATCH",
        body: { busy: false },
      });
      showStatus("Object released.", "success");
      await loadObjects();
    } catch (error) {
      handleActionError(error);
    }
  }

  function openNewCategory() {
    state.editingCategoryId = null;
    elements.categoryForm.reset();
    elements.categoryId.value = "";
    elements.categoryDialogTitle.textContent = "New category";
    elements.categorySave.textContent = "Create category";
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
    elements.categoryDialogTitle.textContent = "Rename category";
    elements.categorySave.textContent = "Save changes";
    hideFormError(elements.categoryError);
    openDialog(elements.categoryDialog);
    elements.categoryName.focus();
  }

  async function deleteObject(id) {
    const object = state.objects.find((item) => item.id === id);
    if (!object) {
      return;
    }
    if (!window.confirm(`Delete "${object.name}"?`)) {
      return;
    }
    try {
      await api(`/api/objects/${encodeURIComponent(id)}`, { method: "DELETE" });
      showStatus("Object deleted.", "success");
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
        `Delete "${category.name}"? Objects in this category will become uncategorized.`,
      )
    ) {
      return;
    }
    try {
      await api(`/api/categories/${encodeURIComponent(id)}`, { method: "DELETE" });
      showStatus("Category deleted. Its objects are now uncategorized.", "success");
      await loadCategories();
      await loadObjects();
    } catch (error) {
      handleActionError(error);
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
        showStatus(`Signed in as ${state.user.email}.`, "success");
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
      showStatus("Signed out.", "success");
    });

    elements.importButton.addEventListener("click", openImport);
    elements.newObjectButton.addEventListener("click", openNewObject);
    elements.newCategoryButton.addEventListener("click", openNewCategory);

    elements.importForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.importError);
      elements.importResult.replaceChildren();
      elements.importResult.hidden = true;
      const file = elements.importFile.files[0];
      if (!file) {
        showFormError(elements.importError, "Choose an .xlsx or CSV file.");
        return;
      }
      const formData = new FormData();
      formData.append("file", file);
      elements.importSave.disabled = true;
      try {
        const result = await api("/api/objects/import", {
          method: "POST",
          body: formData,
        });
        renderImportResult(result);
        if (result.created > 0 || result.updated > 0) {
          await loadCategories();
          await loadObjects();
        }
      } catch (error) {
        handleFormError(error, elements.importError, elements.importDialog);
      } finally {
        elements.importSave.disabled = false;
      }
    });

    elements.objectForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.objectError);
      const name = elements.objectName.value.trim();
      if (!name) {
        showFormError(elements.objectError, "Name is required.");
        elements.objectName.focus();
        return;
      }
      const formData = new FormData();
      formData.append("name", name);
      formData.append("description", elements.objectDescription.value.trim());
      formData.append("category_id", elements.objectCategory.value);
      formData.append("tags", parseTags(elements.objectTags.value).join(","));
      const photo = elements.objectPhoto.files[0];
      if (photo) {
        formData.append("photo", photo);
      } else if (state.editingObjectId && elements.objectRemovePhoto.checked) {
        formData.append("remove_photo", "true");
      }
      elements.objectSave.disabled = true;
      try {
        if (state.editingObjectId) {
          await api(`/api/objects/${encodeURIComponent(state.editingObjectId)}`, {
            method: "PATCH",
            body: formData,
          });
          showStatus("Object updated.", "success");
        } else {
          await api("/api/objects", { method: "POST", body: formData });
          showStatus("Object created.", "success");
        }
        closeDialog(elements.objectDialog);
        await loadObjects();
      } catch (error) {
        handleFormError(error, elements.objectError, elements.objectDialog);
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
        showFormError(elements.busyError, "Choose a time in the future.");
        elements.busyUntil.focus();
        return;
      }
      elements.busySave.disabled = true;
      const fromValue = elements.busyFrom.value;
      const from = fromValue ? new Date(fromValue) : null;
      if (from && (Number.isNaN(from.getTime()) || from.getTime() <= Date.now())) {
        showFormError(elements.busyError, "Choose a reservation start in the future.");
        elements.busyFrom.focus();
        elements.busySave.disabled = false;
        return;
      }
      if (from && from.getTime() >= until.getTime()) {
        showFormError(elements.busyError, "Busy until must be after the reservation start.");
        elements.busyUntil.focus();
        elements.busySave.disabled = false;
        return;
      }
      try {
        if (from) {
          await api(`/api/objects/${encodeURIComponent(elements.busyObjectId.value)}/reservation`, {
            method: "PATCH",
            body: { from: from.toISOString(), until: until.toISOString() },
          });
          showStatus("Object reserved.", "success");
        } else {
          await api(`/api/objects/${encodeURIComponent(elements.busyObjectId.value)}/busy`, {
            method: "PATCH",
            body: { busy: true, busy_until: until.toISOString() },
          });
          showStatus("Object marked busy.", "success");
        }
        closeDialog(elements.busyDialog);
        state.editingBusyId = null;
        await loadObjects();
      } catch (error) {
        handleFormError(error, elements.busyError, elements.busyDialog);
      } finally {
        elements.busySave.disabled = false;
      }
    });

    elements.categoryForm.addEventListener("submit", async (event) => {
      event.preventDefault();
      hideFormError(elements.categoryError);
      const name = elements.categoryName.value.trim();
      if (!name) {
        showFormError(elements.categoryError, "Name is required.");
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
          showStatus("Category updated.", "success");
        } else {
          await api("/api/categories", { method: "POST", body: { name } });
          showStatus("Category created.", "success");
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

    elements.objectPhoto.addEventListener("change", () => {
      hideFormError(elements.objectError);
      const file = elements.objectPhoto.files[0];
      if (!file) {
        elements.objectRemovePhoto.checked = false;
        showPhotoPreview(currentObjectImagePath());
        return;
      }
      if (file.size > 5 * 1024 * 1024) {
        elements.objectPhoto.value = "";
        showFormError(elements.objectError, "Photo must be at most 5 MB.");
        showPhotoPreview(currentObjectImagePath());
        return;
      }
      elements.objectRemovePhoto.checked = false;
      showPhotoPreview(URL.createObjectURL(file), true);
    });

    elements.objectRemovePhoto.addEventListener("change", () => {
      if (elements.objectRemovePhoto.checked) {
        elements.objectPhoto.value = "";
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
    });

    elements.objectGrid.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action]");
      if (!button) {
        return;
      }
      if (button.dataset.action === "edit-object") {
        openEditObject(button.dataset.id);
      } else if (button.dataset.action === "busy-object") {
        openBusyObject(button.dataset.id);
      } else if (button.dataset.action === "release-object") {
        releaseObject(button.dataset.id);
      } else if (button.dataset.action === "delete-object") {
        deleteObject(button.dataset.id);
      }
    });

    elements.categoryList.addEventListener("click", (event) => {
      const button = event.target.closest("button[data-action]");
      if (!button) {
        return;
      }
      if (button.dataset.action === "filter-category") {
        state.filters.category = button.dataset.name;
        state.offset = 0;
        elements.categoryFilter.value = state.filters.category;
        renderCategories();
        loadObjects();
        elements.catalogSection.scrollIntoView({ behavior: "smooth", block: "start" });
      } else if (button.dataset.action === "edit-category") {
        openEditCategory(button.dataset.id);
      } else if (button.dataset.action === "delete-category") {
        deleteCategory(button.dataset.id);
      }
    });
  }

  async function init() {
    bindEvents();
    await loadSession();
    await Promise.all([loadCategories(), loadObjects()]);
  }

  init();
})();
