document.documentElement.classList.add("js-ready");

const themeStorageKey = "homeledger.theme";
const accentStorageKey = "homeledger.accent";

function applyAppearance(theme, accent) {
    document.documentElement.dataset.theme = theme || "system";
    document.documentElement.dataset.accent = accent || "teal";
    updateAppearanceControls();
}

function updateAppearanceControls() {
    const theme = document.documentElement.dataset.theme || "system";
    const accent = document.documentElement.dataset.accent || "teal";

    document.querySelectorAll("[data-theme-choice]").forEach((button) => {
        button.classList.toggle("is-active", button.dataset.themeChoice === theme);
    });
    document.querySelectorAll("[data-accent-choice]").forEach((button) => {
        button.classList.toggle("is-active", button.dataset.accentChoice === accent);
    });
}

document.addEventListener("DOMContentLoaded", () => {
    applyAppearance(
        localStorage.getItem(themeStorageKey) || "system",
        localStorage.getItem(accentStorageKey) || "teal",
    );

    document.querySelectorAll("[data-theme-choice]").forEach((button) => {
        button.addEventListener("click", () => {
            localStorage.setItem(themeStorageKey, button.dataset.themeChoice);
            applyAppearance(button.dataset.themeChoice, document.documentElement.dataset.accent);
        });
    });

    document.querySelectorAll("[data-accent-choice]").forEach((button) => {
        button.addEventListener("click", () => {
            localStorage.setItem(accentStorageKey, button.dataset.accentChoice);
            applyAppearance(document.documentElement.dataset.theme, button.dataset.accentChoice);
        });
    });

    if (window.lucide) {
        window.lucide.createIcons();
    }
});

document.body.addEventListener("htmx:afterSwap", () => {
    updateAppearanceControls();
    if (window.lucide) {
        window.lucide.createIcons();
    }
});

document.body.addEventListener("transaction-created", () => {
    document.querySelector('form[hx-post="/transactions"]')?.reset();
});

document.body.addEventListener("account-created", () => {
    document.querySelector('form[hx-post="/accounts"]')?.reset();
});

document.body.addEventListener("category-created", () => {
    document.querySelector('form[hx-post="/categories"]')?.reset();
});

document.body.addEventListener("account-archived", () => {
    if (window.lucide) {
        window.lucide.createIcons();
    }
});

document.body.addEventListener("category-deleted", () => {
    if (window.lucide) {
        window.lucide.createIcons();
    }
});
