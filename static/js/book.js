// Live price on /book. The server renders the right figure on load and
// recomputes it on submit (pricing.go is the authority); this only keeps the
// number in step as the customer changes their choices. The maths mirrors
// quoteDollars: base + guard, less the plan discount, rounded to whole dollars.
(function () {
    const form = document.getElementById('book-form');
    const out = document.getElementById('price-total');
    if (!form || !out) return;

    const guard = Number(form.dataset.guard) || 0;
    const planPct = Number(form.dataset.planPct) || 0;

    function update() {
        const picked = form.querySelector('input[name="property"]:checked');
        if (!picked) return;
        let price = Number(picked.dataset.price) || 0;
        if (form.querySelector('input[name="guard"]').checked) price += guard;
        if (form.querySelector('input[name="plan"]').checked) price = Math.round(price * (100 - planPct) / 100);
        out.textContent = '$' + price;
    }

    form.addEventListener('change', update);
    update();
})();
