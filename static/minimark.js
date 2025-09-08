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
    const filenameEl = document.getElementById('filename');
    if (!textarea) return;
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
            if (filenameEl) filenameEl.textContent = name;
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
                        currentFilename = newName;
                        if (filenameEl) filenameEl.textContent = newName;
                        document.title = `Minimark - ${newName}`;
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
});
