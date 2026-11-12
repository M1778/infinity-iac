(() => {
    const root = document.documentElement;
    const themeSelect = document.getElementById('theme-select');
    let savedTheme = 'system';
    try {
        const value = window.localStorage.getItem('infinity-iac-docs-theme');
        if (['system', 'light', 'dark'].includes(value)) savedTheme = value;
    } catch (_) {}

    const applyTheme = (theme) => {
        if (theme === 'system') delete root.dataset.theme;
        else root.dataset.theme = theme;
        themeSelect.value = theme;
    };
    applyTheme(savedTheme);
    themeSelect.disabled = false;
    themeSelect.addEventListener('change', () => {
        applyTheme(themeSelect.value);
        try { window.localStorage.setItem('infinity-iac-docs-theme', themeSelect.value); } catch (_) {}
    });

    const menuToggle = document.querySelector('.menu-toggle');
    const sidebar = document.getElementById('sidebar');
    const backdrop = document.querySelector('.sidebar-backdrop');
    const content = document.querySelector('.content');
    const mobile = window.matchMedia('(max-width: 899px)');
    root.dataset.enhanced = '';
    menuToggle.hidden = false;

    const closeMenu = (returnFocus = false) => {
        delete root.dataset.menuOpen;
        backdrop.hidden = true;
        menuToggle.setAttribute('aria-expanded', 'false');
        menuToggle.setAttribute('aria-label', 'Open documentation menu');
        content.inert = false;
        sidebar.inert = mobile.matches;
        if (returnFocus && mobile.matches) menuToggle.focus();
    };
    menuToggle.addEventListener('click', () => {
        if (root.hasAttribute('data-menu-open')) {
            closeMenu(true);
            return;
        }
        sidebar.inert = false;
        root.dataset.menuOpen = '';
        backdrop.hidden = false;
        menuToggle.setAttribute('aria-expanded', 'true');
        menuToggle.setAttribute('aria-label', 'Close documentation menu');
        content.inert = true;
        sidebar.querySelector('a').focus();
    });
    backdrop.addEventListener('click', () => closeMenu(true));
    mobile.addEventListener('change', () => closeMenu(false));
    closeMenu();
    document.addEventListener('keydown', (event) => {
        if (!root.hasAttribute('data-menu-open')) return;
        if (event.key === 'Escape') {
            event.preventDefault();
            closeMenu(true);
        } else if (event.key === 'Tab') {
            const links = [...sidebar.querySelectorAll('a[href], button:not([disabled])')];
            const first = links[0];
            const last = links[links.length - 1];
            if (event.shiftKey && document.activeElement === first) {
                event.preventDefault();
                last.focus();
            } else if (!event.shiftKey && document.activeElement === last) {
                event.preventDefault();
                first.focus();
            }
        }
    });

    const navLinks = [...document.querySelectorAll('.nav-links a')];
    const headings = navLinks.map((link) => document.getElementById(link.hash.slice(1)));
    const currentSection = document.getElementById('current-section');
    const activate = (id) => {
        for (const link of navLinks) {
            if (link.hash === `#${id}`) {
                link.setAttribute('aria-current', 'location');
                currentSection.textContent = link.textContent;
            } else link.removeAttribute('aria-current');
        }
    };
    let scrollPending = false;
    const updateNavigation = () => {
        const offset = document.querySelector('.topbar').getBoundingClientRect().height + 55;
        let active = headings[0];
        for (const heading of headings) {
            if (heading.getBoundingClientRect().top <= offset) active = heading;
            else break;
        }
        activate(active.id);
        scrollPending = false;
    };
    window.addEventListener('scroll', () => {
        if (!scrollPending) {
            scrollPending = true;
            window.requestAnimationFrame(updateNavigation);
        }
    }, { passive: true });
    for (const link of navLinks) {
        link.addEventListener('click', () => {
            closeMenu(false);
            activate(link.hash.slice(1));
        });
    }

    const entries = [...document.querySelectorAll('.recipe-entry')];
    const search = document.getElementById('catalog-search');
    const kind = document.getElementById('catalog-kind');
    const status = document.getElementById('catalog-status');
    const reset = document.getElementById('catalog-reset');
    const result = document.getElementById('catalog-result');
    const empty = document.getElementById('catalog-empty');
    const applyFilters = () => {
        const query = search.value.trim().toLowerCase();
        let visible = 0;
        for (const entry of entries) {
            const matches = (!query || entry.dataset.search.includes(query)) &&
                (kind.value === 'all' || entry.dataset.kind === kind.value) &&
                (status.value === 'all' || entry.dataset.status === status.value);
            entry.hidden = !matches;
            if (matches) visible += 1;
        }
        result.textContent = `Showing ${visible} of ${entries.length} recipes`;
        empty.hidden = visible !== 0;
        reset.disabled = query === '' && kind.value === 'all' && status.value === 'all';
    };
    const resetFilters = () => {
        search.value = '';
        kind.value = 'all';
        status.value = 'all';
        applyFilters();
    };
    for (const control of [search, kind, status]) control.disabled = false;
    reset.hidden = false;
    search.addEventListener('input', applyFilters);
    kind.addEventListener('change', applyFilters);
    status.addEventListener('change', applyFilters);
    reset.addEventListener('click', () => { resetFilters(); search.focus(); });
    applyFilters();

    const revealHash = () => {
        let id;
        try { id = decodeURIComponent(window.location.hash.slice(1)); } catch (_) { return; }
        const target = document.getElementById(id);
        if (target?.classList.contains('recipe-entry')) {
            if (target.hidden) resetFilters();
            target.open = true;
            activate('recipes');
            window.requestAnimationFrame(() => target.scrollIntoView({ block: 'start' }));
        }
    };
    window.addEventListener('hashchange', revealHash);
    revealHash();
    updateNavigation();

    const copyStatus = document.getElementById('copy-status');
    const resetTimers = new WeakMap();
    for (const button of document.querySelectorAll('.copy-button')) {
        button.hidden = false;
        button.addEventListener('click', async () => {
            const pre = button.closest('.code-block').querySelector('pre');
            window.clearTimeout(resetTimers.get(button));
            try {
                if (!navigator.clipboard?.writeText) throw new Error('Clipboard is unavailable');
                await navigator.clipboard.writeText(pre.textContent);
                button.textContent = 'Copied';
                button.dataset.state = 'success';
                copyStatus.textContent = 'Command copied to clipboard.';
            } catch (_) {
                const range = document.createRange();
                range.selectNodeContents(pre);
                const selection = window.getSelection();
                selection.removeAllRanges();
                selection.addRange(range);
                button.textContent = 'Select & copy';
                button.dataset.state = 'error';
                copyStatus.textContent = 'Clipboard unavailable. The command is selected; use your keyboard or browser copy action.';
            }
            resetTimers.set(button, window.setTimeout(() => {
                button.textContent = 'Copy';
                delete button.dataset.state;
            }, 2400));
        });
    }
})();
