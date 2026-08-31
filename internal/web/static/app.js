document.documentElement.classList.add("js-ready");

const iconUploadLimits = {
    maxBytes: 100 * 1024,
    maxDimension: 128,
    allowedTypes: ["image/png", "image/jpeg", "image/svg+xml", "image/webp"],
};

function iconPicker(defaultIcon) {
    return {
        icon: defaultIcon,
        isCustom: false,
        error: "",
        hint: "Oppure carica un'icona: PNG, JPG, SVG o WEBP, max 100 KB, 128×128px.",
        selectIcon(value) {
            this.icon = value;
            this.isCustom = false;
            this.error = "";
        },
        handleUpload(event) {
            const file = event.target.files[0];
            if (!file) {
                return;
            }
            if (!iconUploadLimits.allowedTypes.includes(file.type)) {
                this.error = "Formato non supportato: usa PNG, JPG, SVG o WEBP.";
                event.target.value = "";
                return;
            }
            if (file.size > iconUploadLimits.maxBytes) {
                this.error = "File troppo grande: massimo 100 KB.";
                event.target.value = "";
                return;
            }

            const reader = new FileReader();
            reader.onload = () => {
                if (file.type === "image/svg+xml") {
                    this.icon = reader.result;
                    this.isCustom = true;
                    this.error = "";
                    return;
                }
                const img = new Image();
                img.onload = () => {
                    if (img.width > iconUploadLimits.maxDimension || img.height > iconUploadLimits.maxDimension) {
                        this.error = `Dimensioni massime consentite: ${iconUploadLimits.maxDimension}×${iconUploadLimits.maxDimension}px.`;
                        event.target.value = "";
                        return;
                    }
                    this.icon = reader.result;
                    this.isCustom = true;
                    this.error = "";
                };
                img.onerror = () => {
                    this.error = "Immagine non valida.";
                    event.target.value = "";
                };
                img.src = reader.result;
            };
            reader.onerror = () => {
                this.error = "Non sono riuscito a leggere il file.";
                event.target.value = "";
            };
            reader.readAsDataURL(file);
        },
    };
}

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
