// Override save keystroke to do nothing
document.addEventListener('keydown', function(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        // Do nothing on save
    }
});

// Load index.md into the textarea on page load
let currentFilename = 'index.md';
let currentLock = '';
let saveTimer = null;

window.addEventListener('DOMContentLoaded', async () => {
    const textarea = document.getElementById('typebox');
    const newBtn = document.getElementById('newfile');
    const filepicker = document.getElementById('filepicker');
    const menu = document.getElementById('menu');
    let menuVisible = false;
    let textareaWasDisabled = false;
    let menuSelection = null;
    if (!textarea) return;
    if (menu) {
        const openMenu = () => {
            textareaWasDisabled = textarea.disabled;
            const start = typeof textarea.selectionStart === 'number' ? textarea.selectionStart : textarea.value.length;
            const end = typeof textarea.selectionEnd === 'number' ? textarea.selectionEnd : start;
            menuSelection = { start, end };
            menuVisible = true;
            menu.style.display = 'block';
            textarea.disabled = true;
            textarea.blur();
        };

        const closeMenu = () => {
            if (!menuVisible) return;
            menuVisible = false;
            menu.style.display = 'none';
            if (!textareaWasDisabled) {
                textarea.disabled = false;
                textarea.focus();
            } else {
                textarea.disabled = true;
                textarea.blur();
            }
            menuSelection = null;
            textareaWasDisabled = false;
        };

        const insertImageSnippet = () => {
            const snippet = '![alt text](image-url)';
            const start = menuSelection ? menuSelection.start : (typeof textarea.selectionStart === 'number' ? textarea.selectionStart : textarea.value.length);
            const end = menuSelection ? menuSelection.end : (typeof textarea.selectionEnd === 'number' ? textarea.selectionEnd : start);
            const value = textarea.value;
            const before = value.slice(0, start);
            const after = value.slice(end);
            const caret = start + snippet.length;
            const wasDisabled = textarea.disabled;
            textarea.disabled = false;
            textarea.value = `${before}${snippet}${after}`;
            textarea.setSelectionRange(caret, caret);
            textarea.disabled = wasDisabled;
            closeMenu();
        };

        document.addEventListener('keydown', (event) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                if (menuVisible) {
                    closeMenu();
                } else {
                    openMenu();
                }
                return;
            }

            if (!menuVisible) return;

            if (event.key && event.key.toLowerCase() === 'i') {
                event.preventDefault();
                insertImageSnippet();
            }
        });
    }
    try {
        // Load most recently edited markdown file
        const res = await fetch('/open', { cache: 'no-store' });
        if (!res.ok) {
            textarea.value = '';
            console.warn('Failed to load last markdown file:', res.status);
        } else {
            const text = await res.text();
            textarea.value = text;
            const name = res.headers.get('X-Filename') || 'untitled.md';
            currentFilename = name;
            document.title = `Minimark - ${name}`;
        }
    } catch (err) {
        console.error('Error fetching markdown:', err);
    }

    // Simple lock with 1s TTL, refresh every 500ms
    const setLockedUI = () => {
        textarea.disabled = true;
        textarea.placeholder = 'Locked by another browser tab/window.';
        textarea.title = 'Locked by another browser tab/window.';
    };

    // Try to acquire the lock once
    try {
        const res = await fetch(`/lock?file=${encodeURIComponent(currentFilename)}`, { method: 'POST' });
        if (res.status === 201) {
            currentLock = res.headers.get('X-Lock') || '';
        } else {
            setLockedUI();
            return;
        }
    } catch (err) {
        console.error('Lock error:', err);
        setLockedUI();
        return;
    }

    // Refresh our lock every 500ms; do not auto-reacquire if we lose it
    setInterval(async () => {
        if (!currentLock) return;
        try {
            await fetch(`/lock?file=${encodeURIComponent(currentFilename)}`, {
                method: 'POST',
                headers: { 'X-Lock': currentLock }
            });
        } catch (_) {}
    }, 500);

    // Populate file dropdown
    try {
        const fres = await fetch('/files', { cache: 'no-store' });
        if (fres.ok) {
            const files = await fres.json();
            if (Array.isArray(files) && filepicker) {
                filepicker.innerHTML = '';
                for (const f of files) {
                    const opt = document.createElement('option');
                    opt.value = f; opt.textContent = f;
                    filepicker.appendChild(opt);
                }
                if (currentFilename) {
                    filepicker.value = currentFilename;
                }
            }
        }
    } catch (_) {}

    // Debounced autosave on input (500ms idle)
    textarea.addEventListener('input', () => {
        if (saveTimer) clearTimeout(saveTimer);
        saveTimer = setTimeout(async () => {
            try {
                const res = await fetch(`/save?file=${encodeURIComponent(currentFilename)}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'text/plain; charset=utf-8',
                        'X-Filename': currentFilename,
                        'X-Lock': currentLock,
                    },
                    body: textarea.value
                });
                if (res.status === 204) {
                    const newName = res.headers.get('X-Filename');
                    if (newName && newName !== currentFilename) {
                        const oldName = currentFilename;
                        currentFilename = newName;
                        document.title = `Minimark - ${newName}`;
                        if (filepicker) {
                            let found = false;
                            for (const o of filepicker.options) {
                                if (o.value === oldName) {
                                    o.value = newName; o.textContent = newName; found = true; break;
                                }
                            }
                            if (!found) {
                                const opt = document.createElement('option');
                                opt.value = newName; opt.textContent = newName;
                                filepicker.appendChild(opt);
                            }
                            filepicker.value = newName;
                        }
                    }
                } else if (res.status === 423) {
                    console.warn('File locked by another editor; disabling input.');
                    setLockedUI();
                } else {
                    console.warn('Unexpected save response:', res.status);
                }
            } catch (err) {
                console.error('Autosave failed:', err);
            }
        }, 500);
    });

    // Release lock on unload
    window.addEventListener('beforeunload', async () => {
        if (!currentLock) return;
        try {
            await fetch(`/unlock?file=${encodeURIComponent(currentFilename)}`, {
                method: 'POST',
                headers: { 'X-Lock': currentLock }
            });
        } catch (_) {}
    });

    // Create new untitled.md and open it
    if (newBtn) {
        newBtn.style.cursor = 'pointer';
        newBtn.title = 'New file';
        newBtn.addEventListener('click', async () => {
            // Best-effort unlock current file
            if (currentLock && currentFilename) {
                try {
                    await fetch(`/unlock?file=${encodeURIComponent(currentFilename)}`, {
                        method: 'POST',
                        headers: { 'X-Lock': currentLock },
                    });
                } catch (_) {}
                currentLock = '';
            }
            try {
                const res = await fetch('/new', { method: 'POST' });
                if (!res.ok) {
                    console.warn('Failed to create new file:', res.status);
                    return;
                }
                const newName = (await res.text()).trim();
                currentFilename = newName || 'untitled.md';
                document.title = `Minimark - ${currentFilename}`;
                if (filepicker) {
                    let exists = false;
                    for (const o of filepicker.options) { if (o.value === currentFilename) { exists = true; break; } }
                    if (!exists) {
                        const opt = document.createElement('option');
                        opt.value = currentFilename; opt.textContent = currentFilename;
                        filepicker.appendChild(opt);
                    }
                    filepicker.value = currentFilename;
                }
                // Acquire lock for new file
                const lres = await fetch(`/lock?file=${encodeURIComponent(currentFilename)}`, { method: 'POST' });
                if (lres.status === 201) {
                    currentLock = lres.headers.get('X-Lock') || '';
                } else {
                    // If cannot lock, disable editing
                    textarea.value = '';
                    textarea.disabled = true;
                    textarea.placeholder = 'Locked by another browser tab/window.';
                    textarea.title = 'Locked by another browser tab/window.';
                    return;
                }
                // Start editing the new empty file
                textarea.disabled = false;
                textarea.value = '';
                textarea.placeholder = '';
                textarea.title = '';
                textarea.focus();
            } catch (err) {
                console.error('New file error:', err);
            }
        });
    }

    // Switch file when picker changes
    if (filepicker) {
        filepicker.addEventListener('change', async () => {
            const next = filepicker.value;
            if (!next || next === currentFilename) return;
            // Unlock current
            if (currentLock && currentFilename) {
                try {
                    await fetch(`/unlock?file=${encodeURIComponent(currentFilename)}`, { method: 'POST', headers: { 'X-Lock': currentLock } });
                } catch (_) {}
                currentLock = '';
            }
            try {
                const res = await fetch(`/open?file=${encodeURIComponent(next)}`, { cache: 'no-store' });
                if (!res.ok) { console.warn('Open failed:', res.status); return; }
                const text = await res.text();
                textarea.value = text;
                const name = res.headers.get('X-Filename') || next;
                currentFilename = name;
                document.title = `Minimark - ${name}`;
                // Acquire lock for selected file
                const lres = await fetch(`/lock?file=${encodeURIComponent(currentFilename)}`, { method: 'POST' });
                if (lres.status === 201) {
                    currentLock = lres.headers.get('X-Lock') || '';
                } else {
                    setLockedUI();
                }
            } catch (err) { console.error('Open error:', err); }
        });
    }
});
