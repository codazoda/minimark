// Override save keystroke to do nothing
document.addEventListener('keydown', function(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        // Do nothing on save
    }
});

// Load index.md into the textarea on page load
let currentFilename = 'index.md';
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
                    },
                    body: textarea.value
                });
                const newName = res.headers.get('X-Filename');
                if (newName && newName !== currentFilename) {
                    currentFilename = newName;
                    if (filenameEl) filenameEl.textContent = newName;
                    document.title = `Minimark - ${newName}`;
                }
            } catch (err) {
                console.error('Autosave failed:', err);
            }
        }, 500);
    });
});
