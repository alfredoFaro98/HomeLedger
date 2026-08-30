document.documentElement.classList.add("js-ready");

document.addEventListener("DOMContentLoaded", () => {
    if (window.lucide) {
        window.lucide.createIcons();
    }
});

document.body.addEventListener("htmx:afterSwap", () => {
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
