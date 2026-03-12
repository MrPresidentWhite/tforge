import './style.css';
import './app.css';

import { ListVaults, CreateVault, GetVault, UpdateVault, DeleteVault, ChooseVaultIcon } from '../wailsjs/go/main/App';

const app = document.querySelector('#app');

function createElement(tag, className, children = []) {
    const el = document.createElement(tag);
    if (className) {
        el.className = className;
    }
    for (const child of children) {
        if (typeof child === 'string') {
            el.appendChild(document.createTextNode(child));
        } else if (child instanceof Node) {
            el.appendChild(child);
        }
    }
    return el;
}

let state = {
    vaults: [],
    activeVaultId: null,
    activeEnv: 'dev', // 'dev' | 'staging' | 'prod'
    contextMenu: null, // { x, y, vaultId } | null
    contextMenuEditor: null, // { x, y } | null
    modal: null, // { mode: 'create'|'edit', id?, name, description, icon }
    collapsedGroups: {}, // { [groupPrefix: string]: boolean }
    newEntryPanel: null, // { mode: 'normal' | 'group', prefix: string, suffix: string, selectedGroup?: string } | null
    editingField: null, // { index: number, field: 'key' | 'value' | 'type' } | null
    selectedEntryIndices: new Set(), // Indices in active vault's entries
    convertToGroupModal: null, // { prefix: string, items: { index, currentKey, suffix }[] } | null
    contextMenuDuplicateSub: false, // Submenü "Duplizieren" sichtbar
    duplicateConfirmModal: null, // { target: 'staging'|'prod', keysToOverwrite: string[] } | null
};

function toggleEntrySelection(index) {
    if (state.selectedEntryIndices.has(index)) {
        state.selectedEntryIndices.delete(index);
    } else {
        state.selectedEntryIndices.add(index);
    }
}

function addBulkRow(panel, mode) {
    if (!panel) return;
    if (mode === 'normal') {
        const keys = Array.isArray(panel.keys) ? [...panel.keys] : [''];
        keys.push('');
        state.newEntryPanel = { ...panel, keys, focus: { mode: 'normal', index: keys.length - 1 } };
    } else {
        const suffixes = Array.isArray(panel.suffixes) ? [...panel.suffixes] : [''];
        suffixes.push('');
        state.newEntryPanel = { ...panel, suffixes, focus: { mode: 'group', index: suffixes.length - 1 } };
    }
    render();
}

async function copyToClipboard(text, el) {
    if (!text) return;
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(text);
        } else {
            const temp = document.createElement('textarea');
            temp.value = text;
            document.body.appendChild(temp);
            temp.select();
            document.execCommand('copy');
            document.body.removeChild(temp);
        }
        if (el) {
            el.classList.add('glass-field-copied');
            setTimeout(() => {
                el.classList.remove('glass-field-copied');
            }, 800);
        }
    } catch (err) {
        console.error('Copy to clipboard failed', err);
    }
}

const BULLET_FILL_LENGTH = 80;

function maskValueForDisplay(value, type) {
    if (type === 'secret' && value && value.length > 0) {
        return '•'.repeat(BULLET_FILL_LENGTH);
    }
    return value || '';
}

function createEyeToggleSvg(visible) {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '18');
    svg.setAttribute('height', '18');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    if (visible) {
        svg.innerHTML = '<path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/>';
    } else {
        svg.innerHTML = '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';
    }
    return svg;
}

function createSecretValueDisplay(displayValue, entry, onCopy, onDoubleClick, onCtrlClick) {
    const wrapper = createElement('div', 'glass-field glass-field-secret', []);
    const textSpan = document.createElement('span');
    textSpan.className = 'glass-field-secret-text';
    textSpan.textContent = maskValueForDisplay(displayValue, entry.type);
    wrapper.appendChild(textSpan);

    const eyeBtn = document.createElement('button');
    eyeBtn.type = 'button';
    eyeBtn.className = 'glass-field-eye-toggle';
    eyeBtn.setAttribute('aria-label', 'Anzeigen');
    let isVisible = false;
    eyeBtn.appendChild(createEyeToggleSvg(false));
    eyeBtn.onclick = (e) => {
        e.stopPropagation();
        e.preventDefault();
        isVisible = !isVisible;
        textSpan.textContent = isVisible ? (displayValue || '') : maskValueForDisplay(displayValue, entry.type);
        eyeBtn.replaceChildren(createEyeToggleSvg(isVisible));
        eyeBtn.setAttribute('aria-label', isVisible ? 'Verbergen' : 'Anzeigen');
    };

    wrapper.appendChild(eyeBtn);
    let copyTimeout;
    wrapper.onclick = (e) => {
        if (e.target === eyeBtn || eyeBtn.contains(e.target)) return;
        e.stopPropagation();
        if (e.ctrlKey && onCtrlClick) {
            e.preventDefault();
            onCtrlClick();
            return;
        }
        copyTimeout = setTimeout(() => onCopy(), 250);
    };
    wrapper.ondblclick = (e) => {
        if (e.target === eyeBtn || eyeBtn.contains(e.target)) return;
        clearTimeout(copyTimeout);
        onDoubleClick();
    };
    return wrapper;
}

function splitKeyForGroup(entry) {
    // Gruppierung nur, wenn explizit ein GroupPrefix gesetzt wurde.
    if (!entry || !entry.groupPrefix) {
        return { groupKey: '__UNGROUPED__', prefix: '', suffix: entry?.key || '' };
    }
    const prefix = entry.groupPrefix;
    const fullKey = entry.key || '';
    let suffix = fullKey.startsWith(prefix) ? fullKey.slice(prefix.length) : fullKey;
    return { groupKey: prefix, prefix, suffix };
}

async function loadVaults() {
    try {
        state.vaults = await ListVaults();
        if (state.activeVaultId && !state.vaults.find(v => v.id === state.activeVaultId)) {
            state.activeVaultId = null;
        }
        render();
    } catch (err) {
        console.error('Failed to load vaults', err);
    }
}

function openCreateVaultModal() {
    state.modal = {
        mode: 'create',
        id: null,
        name: '',
        description: '',
        icon: '',
    };
    render();
}

async function handleDeleteVault(id) {
    if (!confirm('Vault wirklich löschen?')) return;
    try {
        await DeleteVault(id);
        state.vaults = state.vaults.filter(v => v.id !== id);
        if (state.activeVaultId === id) {
            state.activeVaultId = null;
        }
        render();
    } catch (err) {
        console.error('Failed to delete vault', err);
    }
}

function openEditVaultModal(id) {
    const existing = state.vaults.find(v => v.id === id);
    if (!existing) return;

    state.modal = {
        mode: 'edit',
        id,
        name: existing.name || '',
        description: existing.description || '',
        icon: existing.icon || '',
    };
    state.contextMenu = null;
    render();
}

function hideContextMenu() {
    if (state.contextMenu || state.contextMenuEditor) {
        state.contextMenu = null;
        state.contextMenuEditor = null;
        state.contextMenuDuplicateSub = false;
        render();
    }
}

async function duplicateDevToTarget(target) {
    const active = state.vaults.find(v => v.id === state.activeVaultId);
    if (!active || !active.entries) return;
    const indices = Array.from(state.selectedEntryIndices);
    if (indices.length === 0) return;
    const entries = [...active.entries];
    const targetKey = target === 'staging' ? 'valueStage' : 'valueProd';
    const sourceKey = 'valueDev';
    indices.forEach(i => {
        if (entries[i]) {
            entries[i] = { ...entries[i], [targetKey]: entries[i][sourceKey] || '' };
        }
    });
    try {
        await UpdateVault({ ...active, entries });
        const fresh = await GetVault(active.id);
        state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
        state.selectedEntryIndices = new Set();
        state.contextMenuEditor = null;
        state.contextMenuDuplicateSub = false;
        state.duplicateConfirmModal = null;
        render();
    } catch (err) {
        console.error('Duplicate failed', err);
    }
}

function openDuplicateToTarget(target) {
    const active = state.vaults.find(v => v.id === state.activeVaultId);
    if (!active || !active.entries) return;
    const indices = Array.from(state.selectedEntryIndices);
    if (indices.length === 0) return;
    const targetKey = target === 'staging' ? 'valueStage' : 'valueProd';
    const keysToOverwrite = indices
        .map(i => active.entries[i])
        .filter(e => e && (e[targetKey] || '').trim() !== '')
        .map(e => e.key || '');
    state.contextMenuEditor = null;
    state.contextMenuDuplicateSub = false;
    if (keysToOverwrite.length > 0) {
        state.duplicateConfirmModal = { target, keysToOverwrite };
    } else {
        duplicateDevToTarget(target);
    }
    render();
}

function renderVaultList() {
    const container = createElement('div', 'vault-list');

    const header = createElement('div', 'vault-list-header', [
        createElement('div', 'vault-list-title', ['Vaults']),
        (() => {
            const btn = createElement('button', 'btn-primary', ['+ Neu']);
            btn.onclick = openCreateVaultModal;
            return btn;
        })(),
    ]);

    const list = createElement('div', 'vault-list-items');
    if (state.vaults.length === 0) {
        list.appendChild(createElement('div', 'vault-list-empty', ['Noch keine Vaults angelegt.']));
    } else {
        for (const v of state.vaults) {
            const hasIcon = v.icon && v.icon.trim();

            const iconNode = hasIcon
                ? (() => {
                    const img = document.createElement('img');
                    img.className = 'vault-list-item-icon-img';
                    img.src = v.icon;
                    img.alt = v.name || 'Vault';
                    img.onerror = () => {
                        img.replaceWith(
                            createElement('div', 'vault-list-item-icon-placeholder', [
                                (v.name && v.name.trim().length > 0 ? v.name.trim()[0].toUpperCase() : 'V'),
                            ])
                        );
                    };
                    return img;
                })()
                : createElement('div', 'vault-list-item-icon-placeholder', [
                    (v.name && v.name.trim().length > 0 ? v.name.trim()[0].toUpperCase() : 'V'),
                ]);

            const item = createElement('div', 'vault-list-item' + (v.id === state.activeVaultId ? ' active' : ''), [
                iconNode,
                createElement('div', 'vault-list-item-main', [
                    createElement('div', 'vault-list-item-name', [v.name]),
                    createElement('div', 'vault-list-item-description', [v.description || '']),
                ]),
            ]);
            item.onclick = () => {
                state.activeVaultId = v.id;
                state.selectedEntryIndices = new Set();
                render();
            };
            item.oncontextmenu = (e) => {
                e.preventDefault();
                e.stopPropagation();
                state.contextMenu = {
                    x: e.clientX,
                    y: e.clientY,
                    vaultId: v.id,
                };
                render();
            };
            list.appendChild(item);
        }
    }

    container.appendChild(header);
    container.appendChild(list);
    return container;
}

function renderVaultEditor() {
    const container = createElement('div', 'vault-editor');
    const active = state.vaults.find(v => v.id === state.activeVaultId);

    if (!active) {
        container.appendChild(createElement('div', 'vault-editor-empty', [
            'Wähle links einen Vault aus oder erstelle einen neuen.',
        ]));
        return container;
    }

    const titleRow = createElement('div', 'vault-editor-header', [
        createElement('div', 'vault-editor-title', [active.name]),
        (() => {
            const btn = createElement('button', 'btn-danger', ['Löschen']);
            btn.onclick = () => handleDeleteVault(active.id);
            return btn;
        })(),
    ]);

    const envSwitcher = createElement('div', 'env-switcher', []);
    ['dev', 'staging', 'prod'].forEach(env => {
        const labelMap = { dev: 'DEV', staging: 'STAGING', prod: 'PROD' };
        const pill = createElement('button', 'env-pill' + (state.activeEnv === env ? ' active' : ''), [
            labelMap[env],
        ]);
        pill.onclick = () => {
            state.activeEnv = env;
            render();
        };
        envSwitcher.appendChild(pill);
    });

    container.appendChild(titleRow);
    container.appendChild(envSwitcher);

    const entries = active.entries || [];

    // Gruppierung nach explizitem GroupPrefix
    const groups = {};
    entries.forEach((entry, index) => {
        const { groupKey, prefix, suffix } = splitKeyForGroup(entry);
        if (!groups[groupKey]) {
            groups[groupKey] = { prefix, items: [] };
        }
        groups[groupKey].items.push({ entry, index, suffix });
    });

    // Reihenfolge der Gruppen wie angelegt
    const groupOrder = Object.keys(groups);

    function createHeaderRow() {
        return createElement('div', 'vault-editor-row vault-editor-row-header', [
            createElement('div', 'col-key', ['Key']),
            createElement('div', 'col-value', [
                'Value ',
                (() => {
                    const span = createElement('span', 'env-label-header', [
                        state.activeEnv === 'dev' ? 'DEV' : state.activeEnv === 'staging' ? 'STAGING' : 'PROD',
                    ]);
                    return span;
                })(),
            ]),
            createElement('div', 'col-type', ['Typ']),
            createElement('div', 'col-actions', ['']),
        ]);
    }

    // Alle Gruppen in einer Card mit globalem Header
    const groupedKeys = groupOrder.filter(gk => gk !== '__UNGROUPED__');
    if (groupedKeys.length > 0) {
        const groupsCard = createElement('div', 'vault-editor-table vault-editor-table-groups', []);
        groupsCard.appendChild(createElement('div', 'vault-editor-section-title', ['Gruppen']));
        groupsCard.appendChild(createHeaderRow());

        groupedKeys.forEach(gk => {
            const group = groups[gk];
            const isUngrouped = false;

            const collapsed = state.collapsedGroups[gk] !== false;
            const groupHeader = createElement('div', 'vault-editor-group-header', []);
            const toggleBtn = createElement('button', 'group-toggle-btn', [
                collapsed ? '▶' : '▼',
            ]);
            toggleBtn.onclick = () => {
                state.collapsedGroups[gk] = !collapsed;
                render();
            };
            const title = createElement('div', 'group-title', [group.prefix.replace(/_$/, '')]);
            groupHeader.appendChild(toggleBtn);
            groupHeader.appendChild(title);
            groupsCard.appendChild(groupHeader);

            if (!collapsed) {
                group.items.forEach(({ entry, index, suffix }) => {
                    const row = createElement('div', 'vault-editor-row' + (state.selectedEntryIndices.has(index) ? ' vault-editor-row-selected' : ''), []);
                    row.oncontextmenu = (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        state.contextMenuEditor = { x: e.clientX, y: e.clientY };
                        render();
                    };

            const isEditingKey = state.editingField && state.editingField.index === index && state.editingField.field === 'key';
            const isEditingValue = state.editingField && state.editingField.index === index && state.editingField.field === 'value';
            const isEditingType = state.editingField && state.editingField.index === index && state.editingField.field === 'type';

            const keyWrapper = createElement('div', 'key-group-wrapper', []);
            let keyInput;
            if (isEditingKey) {
                if (!isUngrouped && group.prefix) {
                    keyInput = createElement('input', 'input key-suffix-input');
                    keyInput.value = suffix;
                } else {
                    keyInput = createElement('input', 'input');
                    keyInput.value = entry.key;
                }
                keyWrapper.appendChild(keyInput);
            } else {
                const displayText = (!isUngrouped && group.prefix) ? (suffix || '') : (entry.key || '');
                const keyDisplay = createElement('div', 'glass-field', [displayText || '']);
                let keyCopyTimeout;
                keyDisplay.onclick = (e) => {
                    e.stopPropagation();
                    if (e.ctrlKey) {
                        toggleEntrySelection(index);
                        e.preventDefault();
                        render();
                        return;
                    }
                    const fullKeyForCopy = (!isUngrouped && group.prefix)
                        ? (group.prefix + (suffix || ''))
                        : (entry.key || '');
                    keyCopyTimeout = setTimeout(() => copyToClipboard(fullKeyForCopy, keyDisplay), 250);
                };
                keyDisplay.ondblclick = () => {
                    clearTimeout(keyCopyTimeout);
                    state.editingField = { index, field: 'key' };
                    render();
                };
                keyWrapper.appendChild(keyDisplay);
            }

            let valueInput;
            if (isEditingValue) {
                valueInput = createElement('input', 'input');
                if (state.activeEnv === 'dev') {
                    valueInput.value = entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    valueInput.value = entry.valueStage || '';
                } else {
                    valueInput.value = entry.valueProd || '';
                }
            }

            let typeSelect;
            if (isEditingType) {
                typeSelect = createElement('select', 'select');
                ['env', 'secret', 'note'].forEach(t => {
                    const opt = createElement('option', '', [t]);
                    opt.value = t;
                    if (t === entry.type) {
                        opt.selected = true;
                    }
                    typeSelect.appendChild(opt);
                });
            }

            const deleteBtn = createElement('button', 'btn-danger btn-small', ['✕']);
            deleteBtn.onclick = async () => {
                const updated = { ...active };
                const newEntries = [...entries];
                newEntries.splice(index, 1);
                updated.entries = newEntries;
                try {
                    await UpdateVault(updated);
                    const fresh = await GetVault(active.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                    render();
                } catch (err) {
                    console.error('Failed to update vault', err);
                }
            };

            const handleChange = async () => {
                const updated = { ...active };
                const newEntries = [...entries];
                const existing = newEntries[index] || {};
                const fullKey = (!isUngrouped && group.prefix)
                    ? (group.prefix + (keyInput ? (keyInput.value || '') : suffix || ''))
                    : (keyInput ? keyInput.value : entry.key);

                const nextEntry = {
                    key: fullKey,
                    valueDev: existing.valueDev || '',
                    valueStage: existing.valueStage || '',
                    valueProd: existing.valueProd || '',
                    type: typeSelect ? typeSelect.value : existing.type,
                    groupPrefix: (!isUngrouped && group.prefix) ? group.prefix : '',
                };
                if (state.activeEnv === 'dev') {
                    nextEntry.valueDev = valueInput ? valueInput.value : entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    nextEntry.valueStage = valueInput ? valueInput.value : entry.valueStage || '';
                } else {
                    nextEntry.valueProd = valueInput ? valueInput.value : entry.valueProd || '';
                }
                newEntries[index] = nextEntry;
                updated.entries = newEntries;
                try {
                    await UpdateVault(updated);
                    const fresh = await GetVault(active.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                    state.editingField = null;
                    render();
                } catch (err) {
                    console.error('Failed to update vault', err);
                }
            };

            if (isEditingKey && keyInput) {
                keyInput.onblur = handleChange;
                keyInput.onkeydown = (e) => {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        handleChange();
                    }
                };
                setTimeout(() => keyInput.focus(), 0);
            }

            const valueCol = (() => {
                if (isEditingValue && valueInput) {
                    valueInput.onblur = handleChange;
                    valueInput.onkeydown = (e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            handleChange();
                        }
                    };
                    setTimeout(() => valueInput.focus(), 0);
                    return createElement('div', 'col-value', [valueInput]);
                }
                let displayValue = '';
                if (state.activeEnv === 'dev') {
                    displayValue = entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    displayValue = entry.valueStage || '';
                } else {
                    displayValue = entry.valueProd || '';
                }
                let valueDisplay;
                if (entry.type === 'secret') {
                    valueDisplay = createSecretValueDisplay(displayValue, entry,
                        () => copyToClipboard(displayValue || '', valueDisplay),
                        () => { state.editingField = { index, field: 'value' }; render(); },
                        () => { toggleEntrySelection(index); render(); });
                } else {
                    valueDisplay = createElement('div', 'glass-field', [maskValueForDisplay(displayValue, entry.type) || '']);
                    let valueCopyTimeout;
                    valueDisplay.onclick = (e) => {
                        e.stopPropagation();
                        if (e.ctrlKey) {
                            toggleEntrySelection(index);
                            e.preventDefault();
                            render();
                            return;
                        }
                        valueCopyTimeout = setTimeout(() => copyToClipboard(displayValue || '', valueDisplay), 250);
                    };
                    valueDisplay.ondblclick = () => {
                        clearTimeout(valueCopyTimeout);
                        state.editingField = { index, field: 'value' };
                        render();
                    };
                }
                return createElement('div', 'col-value', [valueDisplay]);
            })();

            const typeCol = (() => {
                if (isEditingType && typeSelect) {
                    typeSelect.onchange = handleChange;
                    typeSelect.onblur = () => {
                        state.editingField = null;
                        render();
                    };
                    return createElement('div', 'col-type', [typeSelect]);
                }
                const typeDisplay = createElement('div', 'glass-field', [entry.type || 'env']);
                typeDisplay.ondblclick = () => {
                    state.editingField = { index, field: 'type' };
                    render();
                };
                return createElement('div', 'col-type', [typeDisplay]);
            })();

            row.appendChild(createElement('div', 'col-key', [keyWrapper]));
            row.appendChild(valueCol);
            row.appendChild(typeCol);
            row.appendChild(createElement('div', 'col-actions', [deleteBtn]));
            groupsCard.appendChild(row);
                });
            }
        });

        container.appendChild(groupsCard);
    }

    // Für einzelne (ungegroupte) Keys: eine gemeinsame Card mit globalem Header
    const ungroupedGroup = groups['__UNGROUPED__'];
    if (ungroupedGroup && ungroupedGroup.items.length > 0) {
        const card = createElement('div', 'vault-editor-table', []);
        card.appendChild(createElement('div', 'vault-editor-section-title', ['Einzelne Keys']));
        card.appendChild(createHeaderRow());

        ungroupedGroup.items.forEach(({ entry, index, suffix }) => {
            const row = createElement('div', 'vault-editor-row' + (state.selectedEntryIndices.has(index) ? ' vault-editor-row-selected' : ''), []);
            row.oncontextmenu = (e) => {
                e.preventDefault();
                e.stopPropagation();
                state.contextMenuEditor = { x: e.clientX, y: e.clientY };
                render();
            };

            const isEditingKey = state.editingField && state.editingField.index === index && state.editingField.field === 'key';
            const isEditingValue = state.editingField && state.editingField.index === index && state.editingField.field === 'value';
            const isEditingType = state.editingField && state.editingField.index === index && state.editingField.field === 'type';

            const keyWrapper = createElement('div', 'key-group-wrapper', []);
            let keyInput;
            if (isEditingKey) {
                keyInput = createElement('input', 'input');
                keyInput.value = entry.key;
                keyWrapper.appendChild(keyInput);
            } else {
                const displayText = entry.key || '';
                const keyDisplay = createElement('div', 'glass-field', [displayText || '']);
                let keyCopyTimeout;
                keyDisplay.onclick = (e) => {
                    e.stopPropagation();
                    if (e.ctrlKey) {
                        toggleEntrySelection(index);
                        e.preventDefault();
                        render();
                        return;
                    }
                    const fullKeyForCopy = entry.key || '';
                    keyCopyTimeout = setTimeout(() => copyToClipboard(fullKeyForCopy, keyDisplay), 250);
                };
                keyDisplay.ondblclick = () => {
                    clearTimeout(keyCopyTimeout);
                    state.editingField = { index, field: 'key' };
                    render();
                };
                keyWrapper.appendChild(keyDisplay);
            }

            let valueInput;
            if (isEditingValue) {
                valueInput = createElement('input', 'input');
                if (state.activeEnv === 'dev') {
                    valueInput.value = entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    valueInput.value = entry.valueStage || '';
                } else {
                    valueInput.value = entry.valueProd || '';
                }
            }

            let typeSelect;
            if (isEditingType) {
                typeSelect = createElement('select', 'select');
                ['env', 'secret', 'note'].forEach(t => {
                    const opt = createElement('option', '', [t]);
                    opt.value = t;
                    if (t === entry.type) {
                        opt.selected = true;
                    }
                    typeSelect.appendChild(opt);
                });
            }

            const deleteBtn = createElement('button', 'btn-danger btn-small', ['✕']);
            deleteBtn.onclick = async () => {
                const updated = { ...active };
                const newEntries = [...entries];
                newEntries.splice(index, 1);
                updated.entries = newEntries;
                try {
                    await UpdateVault(updated);
                    const fresh = await GetVault(active.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                    render();
                } catch (err) {
                    console.error('Failed to update vault', err);
                }
            };

            const handleChange = async () => {
                const updated = { ...active };
                const newEntries = [...entries];
                const existing = newEntries[index] || {};
                const fullKey = keyInput ? keyInput.value : entry.key;

                const nextEntry = {
                    key: fullKey,
                    valueDev: existing.valueDev || '',
                    valueStage: existing.valueStage || '',
                    valueProd: existing.valueProd || '',
                    type: typeSelect ? typeSelect.value : existing.type,
                    groupPrefix: '',
                };
                if (state.activeEnv === 'dev') {
                    nextEntry.valueDev = valueInput ? valueInput.value : entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    nextEntry.valueStage = valueInput ? valueInput.value : entry.valueStage || '';
                } else {
                    nextEntry.valueProd = valueInput ? valueInput.value : entry.valueProd || '';
                }
                newEntries[index] = nextEntry;
                updated.entries = newEntries;
                try {
                    await UpdateVault(updated);
                    const fresh = await GetVault(active.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                    state.editingField = null;
                    render();
                } catch (err) {
                    console.error('Failed to update vault', err);
                }
            };

            if (isEditingKey && keyInput) {
                keyInput.onblur = handleChange;
                keyInput.onkeydown = (e) => {
                    if (e.key === 'Enter') {
                        e.preventDefault();
                        handleChange();
                    }
                };
                setTimeout(() => keyInput.focus(), 0);
            }

            const valueCol = (() => {
                if (isEditingValue && valueInput) {
                    valueInput.onblur = handleChange;
                    valueInput.onkeydown = (e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            handleChange();
                        }
                    };
                    setTimeout(() => valueInput.focus(), 0);
                    return createElement('div', 'col-value', [valueInput]);
                }
                let displayValue = '';
                if (state.activeEnv === 'dev') {
                    displayValue = entry.valueDev || '';
                } else if (state.activeEnv === 'staging') {
                    displayValue = entry.valueStage || '';
                } else {
                    displayValue = entry.valueProd || '';
                }
                let valueDisplay;
                if (entry.type === 'secret') {
                    valueDisplay = createSecretValueDisplay(displayValue, entry,
                        () => copyToClipboard(displayValue || '', valueDisplay),
                        () => { state.editingField = { index, field: 'value' }; render(); },
                        () => { toggleEntrySelection(index); render(); });
                } else {
                    valueDisplay = createElement('div', 'glass-field', [maskValueForDisplay(displayValue, entry.type) || '']);
                    let valueCopyTimeout;
                    valueDisplay.onclick = (e) => {
                        e.stopPropagation();
                        if (e.ctrlKey) {
                            toggleEntrySelection(index);
                            e.preventDefault();
                            render();
                            return;
                        }
                        valueCopyTimeout = setTimeout(() => copyToClipboard(displayValue || '', valueDisplay), 250);
                    };
                    valueDisplay.ondblclick = () => {
                        clearTimeout(valueCopyTimeout);
                        state.editingField = { index, field: 'value' };
                        render();
                    };
                }
                return createElement('div', 'col-value', [valueDisplay]);
            })();

            const typeCol = (() => {
                if (isEditingType && typeSelect) {
                    typeSelect.onchange = handleChange;
                    typeSelect.onblur = () => {
                        state.editingField = null;
                        render();
                    };
                    return createElement('div', 'col-type', [typeSelect]);
                }
                const typeDisplay = createElement('div', 'glass-field', [entry.type || 'env']);
                typeDisplay.ondblclick = () => {
                    state.editingField = { index, field: 'type' };
                    render();
                };
                return createElement('div', 'col-type', [typeDisplay]);
            })();

            row.appendChild(createElement('div', 'col-key', [keyWrapper]));
            row.appendChild(valueCol);
            row.appendChild(typeCol);
            row.appendChild(createElement('div', 'col-actions', [deleteBtn]));
            card.appendChild(row);
        });
        container.appendChild(card);
    }

    const addRowBtn = createElement('button', 'btn-secondary', ['+ Eintrag hinzufügen']);
    addRowBtn.onclick = () => {
        state.newEntryPanel = state.newEntryPanel
            ? null
            : { mode: 'normal', prefix: '', selectedGroup: '', keys: [''], suffixes: [''], focus: { mode: 'normal', index: 0 } };
        render();
    };

    const addPanel = (() => {
        if (!state.newEntryPanel) return null;
        const wrap = createElement('div', 'new-entry-panel', []);

        const modeToggle = createElement('div', 'new-entry-mode-toggle', []);
        const normalBtn = createElement(
            'button',
            'chip' + (state.newEntryPanel.mode === 'normal' ? ' chip-active' : ''),
            ['Normaler Key']
        );
        const groupBtn = createElement(
            'button',
            'chip' + (state.newEntryPanel.mode === 'group' ? ' chip-active' : ''),
            ['Gruppe (Prefix)']
        );
        normalBtn.onclick = () => {
            state.newEntryPanel = { ...state.newEntryPanel, mode: 'normal' };
            render();
        };
        groupBtn.onclick = () => {
            state.newEntryPanel = { ...state.newEntryPanel, mode: 'group' };
            render();
        };
        modeToggle.appendChild(normalBtn);
        modeToggle.appendChild(groupBtn);

        wrap.appendChild(modeToggle);

        const formRow = createElement('div', 'new-entry-form-row', []);
        if (state.newEntryPanel.mode === 'group') {
            // existierende Gruppen-Prefixe sammeln
            const existingGroupPrefixes = [];
            const seen = new Set();
            (active.entries || []).forEach(e => {
                if (e.groupPrefix && !seen.has(e.groupPrefix)) {
                    seen.add(e.groupPrefix);
                    existingGroupPrefixes.push(e.groupPrefix);
                }
            });

            const hasExistingGroups = existingGroupPrefixes.length > 0;
            const CUSTOM_VALUE = '__CUSTOM__';

            const groupCol = createElement('div', 'new-entry-group-col', []);

            if (hasExistingGroups) {
                const select = createElement('select', 'select new-entry-group-select');

                const currentSelected = state.newEntryPanel.selectedGroup || existingGroupPrefixes[0];
                // wenn noch kein Prefix gesetzt ist, initial auf Auswahl setzen
                if (!state.newEntryPanel.prefix && currentSelected && currentSelected !== CUSTOM_VALUE) {
                    state.newEntryPanel.prefix = currentSelected;
                }

                existingGroupPrefixes.forEach(prefix => {
                    const opt = createElement('option', '', [prefix.replace(/_$/, '')]);
                    opt.value = prefix;
                    if (prefix === currentSelected) {
                        opt.selected = true;
                    }
                    select.appendChild(opt);
                });

                const customOpt = createElement('option', '', ['CUSTOM']);
                customOpt.value = CUSTOM_VALUE;
                if (currentSelected === CUSTOM_VALUE) {
                    customOpt.selected = true;
                }
                select.appendChild(customOpt);

                select.onchange = (e) => {
                    const val = e.target.value;
                    state.newEntryPanel = {
                        ...state.newEntryPanel,
                        selectedGroup: val,
                    };
                    if (val !== CUSTOM_VALUE) {
                        state.newEntryPanel.prefix = val;
                    }
                    render();
                };

                groupCol.appendChild(select);

                const isCustom = (state.newEntryPanel.selectedGroup || currentSelected) === CUSTOM_VALUE;
                if (isCustom) {
                    const prefixInput = createElement('input', 'input new-entry-prefix');
                    prefixInput.placeholder = 'Prefix, z.B. POSTGRES_';
                    prefixInput.value = state.newEntryPanel.prefix || '';
                    prefixInput.oninput = (e) => {
                        state.newEntryPanel = { ...state.newEntryPanel, prefix: e.target.value };
                    };
                    groupCol.appendChild(prefixInput);
                }
            } else {
                // keine existierenden Gruppen -> wie vorher nur Prefix-Eingabe
                const prefixInput = createElement('input', 'input new-entry-prefix');
                prefixInput.placeholder = 'Prefix, z.B. POSTGRES_';
                prefixInput.value = state.newEntryPanel.prefix || '';
                prefixInput.oninput = (e) => {
                    state.newEntryPanel = { ...state.newEntryPanel, prefix: e.target.value };
                };
                groupCol.appendChild(prefixInput);
            }

            formRow.appendChild(groupCol);
            const suffixList = createElement('div', 'new-entry-keys-list', []);
            const suffixes = Array.isArray(state.newEntryPanel.suffixes) ? state.newEntryPanel.suffixes : [state.newEntryPanel.suffix || ''];
            suffixes.forEach((val, i) => {
                const suffixInput = createElement('input', 'input new-entry-suffix');
                suffixInput.placeholder = 'Key in Gruppe, z.B. HOST';
                suffixInput.value = val || '';
                suffixInput.oninput = (e) => {
                    const next = [...suffixes];
                    next[i] = e.target.value;
                    state.newEntryPanel = { ...state.newEntryPanel, suffixes: next };
                };
                suffixInput.onkeydown = (e) => {
                    if (e.key === 'Enter' && e.shiftKey) {
                        e.preventDefault();
                        addBulkRow(state.newEntryPanel, 'group');
                    }
                };
                if (state.newEntryPanel.focus && state.newEntryPanel.focus.mode === 'group' && state.newEntryPanel.focus.index === i) {
                    setTimeout(() => suffixInput.focus(), 0);
                }
                suffixList.appendChild(suffixInput);
            });
            formRow.appendChild(suffixList);
        } else {
            const list = createElement('div', 'new-entry-keys-list new-entry-keys-list-full', []);
            const keys = Array.isArray(state.newEntryPanel.keys) ? state.newEntryPanel.keys : [state.newEntryPanel.suffix || ''];
            keys.forEach((val, i) => {
                const keyInput = createElement('input', 'input new-entry-key');
                keyInput.placeholder = 'Key, z.B. NEXT_PUBLIC_API_URL';
                keyInput.value = val || '';
                keyInput.oninput = (e) => {
                    const next = [...keys];
                    next[i] = e.target.value;
                    state.newEntryPanel = { ...state.newEntryPanel, keys: next };
                };
                keyInput.onkeydown = (e) => {
                    if (e.key === 'Enter' && e.shiftKey) {
                        e.preventDefault();
                        addBulkRow(state.newEntryPanel, 'normal');
                    }
                };
                if (state.newEntryPanel.focus && state.newEntryPanel.focus.mode === 'normal' && state.newEntryPanel.focus.index === i) {
                    setTimeout(() => keyInput.focus(), 0);
                }
                list.appendChild(keyInput);
            });
            formRow.appendChild(list);
        }

        const createBtn = createElement('button', 'btn-primary new-entry-create-btn', ['Erstellen']);
        createBtn.onclick = async () => {
            const panel = state.newEntryPanel;
            if (!panel) return;

            const updated = { ...active };
            const newEntries = [...(active.entries || [])];

            if (panel.mode === 'group') {
                const finalPrefix = (panel.prefix || '').trim();
                const suffixes = Array.isArray(panel.suffixes) ? panel.suffixes : [(panel.suffix || '')];
                const cleanSuffixes = suffixes.map(s => (s || '').trim()).filter(Boolean);
                if (!finalPrefix || cleanSuffixes.length === 0) return;
                cleanSuffixes.forEach(suf => {
                    newEntries.push({
                        key: finalPrefix + suf,
                        valueDev: '',
                        valueStage: '',
                        valueProd: '',
                        type: 'secret',
                        groupPrefix: finalPrefix,
                    });
                });
            } else {
                const keys = Array.isArray(panel.keys) ? panel.keys : [(panel.suffix || '')];
                const cleanKeys = keys.map(k => (k || '').trim()).filter(Boolean);
                if (cleanKeys.length === 0) return;
                cleanKeys.forEach(key => {
                    newEntries.push({
                        key,
                        valueDev: '',
                        valueStage: '',
                        valueProd: '',
                        type: 'secret',
                        groupPrefix: '',
                    });
                });
            }

            updated.entries = newEntries;
            try {
                await UpdateVault(updated);
                const fresh = await GetVault(active.id);
                state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                state.newEntryPanel = null;
                render();
            } catch (err) {
                console.error('Failed to update vault', err);
            }
        };

        wrap.appendChild(formRow);
        wrap.appendChild(createBtn);
        return wrap;
    })();

    container.appendChild(addRowBtn);
    if (addPanel) {
        container.appendChild(addPanel);
    }
    return container;
}

function render() {
    app.innerHTML = '';

    const shell = createElement('div', 'vault-shell');

    const left = renderVaultList();
    const right = renderVaultEditor();

    shell.appendChild(left);
    shell.appendChild(right);

    app.appendChild(shell);

    if (state.contextMenu) {
        const menu = createElement('div', 'context-menu', []);
        menu.style.left = `${state.contextMenu.x}px`;
        menu.style.top = `${state.contextMenu.y}px`;

        const editItem = createElement('div', 'context-menu-item', ['Bearbeiten']);
        editItem.onclick = () => openEditVaultModal(state.contextMenu.vaultId);

        const deleteItem = createElement('div', 'context-menu-item context-menu-item-danger', ['Löschen']);
        deleteItem.onclick = () => {
            const id = state.contextMenu.vaultId;
            state.contextMenu = null;
            render();
            handleDeleteVault(id);
        };

        menu.appendChild(editItem);
        menu.appendChild(deleteItem);
        app.appendChild(menu);
    }

    if (state.contextMenuEditor) {
        const wrapper = createElement('div', 'context-menu-wrapper', []);
        wrapper.style.left = `${state.contextMenuEditor.x}px`;
        wrapper.style.top = `${state.contextMenuEditor.y}px`;
        wrapper.onmouseleave = () => {
            state.contextMenuDuplicateSub = false;
            render();
        };

        const menu = createElement('div', 'context-menu', []);

        const hasSelection = state.selectedEntryIndices && state.selectedEntryIndices.size > 0;
        const convertItem = createElement('div', 'context-menu-item' + (!hasSelection ? ' context-menu-item-disabled' : ''), ['In Gruppe konvertieren']);
        convertItem.onclick = () => {
            if (!hasSelection) return;
            const active = state.vaults.find(v => v.id === state.activeVaultId);
            if (!active || !active.entries) return;
            const indices = Array.from(state.selectedEntryIndices).sort((a, b) => a - b);
            state.convertToGroupModal = {
                prefix: '',
                items: indices.map(i => ({ index: i, currentKey: active.entries[i].key || '', suffix: active.entries[i].key || '' })),
            };
            state.contextMenuEditor = null;
            render();
        };

        const duplicateItem = createElement('div', 'context-menu-item context-menu-item-with-sub' + (!hasSelection ? ' context-menu-item-disabled' : ''), ['Duplizieren']);
        duplicateItem.onmouseenter = () => {
            if (!hasSelection) return;
            state.contextMenuDuplicateSub = true;
            render();
        };
        duplicateItem.onclick = (e) => { e.stopPropagation(); };

        const clearItem = createElement('div', 'context-menu-item', ['Auswahl aufheben']);
        clearItem.onclick = () => {
            state.selectedEntryIndices = new Set();
            state.contextMenuEditor = null;
            render();
        };

        menu.appendChild(convertItem);
        menu.appendChild(duplicateItem);
        menu.appendChild(clearItem);
        wrapper.appendChild(menu);

        if (state.contextMenuDuplicateSub) {
            const sub = createElement('div', 'context-menu context-menu-sub', []);
            const stagingItem = createElement('div', 'context-menu-item', ['Nach STAGING']);
            stagingItem.onclick = () => openDuplicateToTarget('staging');
            const prodItem = createElement('div', 'context-menu-item', ['Nach PROD']);
            prodItem.onclick = () => openDuplicateToTarget('prod');
            sub.appendChild(stagingItem);
            sub.appendChild(prodItem);
            wrapper.appendChild(sub);
        }

        app.appendChild(wrapper);
    }

    if (state.modal) {
        const overlay = createElement('div', 'modal-overlay', []);
        const modal = createElement('div', 'modal', []);

        const titleText = state.modal.mode === 'create' ? 'Neuen Vault erstellen' : 'Vault bearbeiten';
        const title = createElement('div', 'modal-title', [titleText]);

        const form = createElement('div', 'modal-form', []);

        const nameLabel = createElement('label', 'modal-label', ['Name']);
        const nameInput = createElement('input', 'input modal-input', []);
        nameInput.value = state.modal.name;
        nameInput.oninput = (e) => {
            state.modal.name = e.target.value;
        };

        const descLabel = createElement('label', 'modal-label', ['Beschreibung']);
        const descInput = createElement('textarea', 'input modal-textarea', []);
        descInput.value = state.modal.description;
        descInput.oninput = (e) => {
            state.modal.description = e.target.value;
        };

        const iconRow = createElement('div', 'modal-icon-row', []);
        const iconLabel = createElement('div', 'modal-label', ['Icon (optional)']);
        const iconPreview = state.modal.icon
            ? (() => {
                const img = document.createElement('img');
                img.className = 'vault-list-item-icon-img modal-icon-preview';
                img.src = state.modal.icon;
                img.alt = 'Vault Icon';
                return img;
            })()
            : createElement('div', 'vault-list-item-icon-placeholder modal-icon-placeholder', [
                (state.modal.name && state.modal.name.trim().length > 0
                    ? state.modal.name.trim()[0].toUpperCase()
                    : 'V'),
            ]);

        const iconButton = createElement('button', 'btn-secondary modal-icon-button', ['Icon wählen']);
        iconButton.onclick = async (e) => {
            e.preventDefault();
            try {
                const iconPath = await ChooseVaultIcon();
                if (iconPath) {
                    state.modal.icon = iconPath;
                    render();
                }
            } catch (err) {
                console.error('Choose icon failed', err);
            }
        };

        iconRow.appendChild(iconPreview);
        iconRow.appendChild(iconButton);

        form.appendChild(nameLabel);
        form.appendChild(nameInput);
        form.appendChild(descLabel);
        form.appendChild(descInput);
        form.appendChild(iconLabel);
        form.appendChild(iconRow);

        const actions = createElement('div', 'modal-actions', []);
        const cancelBtn = createElement('button', 'btn-secondary', ['Abbrechen']);
        cancelBtn.onclick = (e) => {
            e.preventDefault();
            state.modal = null;
            render();
        };

        const saveBtn = createElement('button', 'btn-primary', ['Speichern']);
        saveBtn.onclick = async (e) => {
            e.preventDefault();
            const name = (state.modal.name || '').trim();
            if (!name) {
                return;
            }
            const description = state.modal.description || '';
            const icon = state.modal.icon || '';

            try {
                if (state.modal.mode === 'create') {
                    const v = await CreateVault(name, description);
                    if (icon) {
                        v.icon = icon;
                        await UpdateVault(v);
                    }
                    state.vaults.push(v);
                    state.activeVaultId = v.id;
                } else if (state.modal.mode === 'edit') {
                    const existing = state.vaults.find(v => v.id === state.modal.id);
                    if (!existing) {
                        state.modal = null;
                        render();
                        return;
                    }
                    const updated = { ...existing, name, description, icon };
                    await UpdateVault(updated);
                    const fresh = await GetVault(existing.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                }
                state.modal = null;
                render();
            } catch (err) {
                console.error('Failed to save vault', err);
            }
        };

        actions.appendChild(cancelBtn);
        actions.appendChild(saveBtn);

        modal.appendChild(title);
        modal.appendChild(form);
        modal.appendChild(actions);

        overlay.appendChild(modal);
        overlay.onclick = (e) => {
            if (e.target === overlay) {
                state.modal = null;
                render();
            }
        };

        app.appendChild(overlay);
    }

    if (state.convertToGroupModal) {
        const active = state.vaults.find(v => v.id === state.activeVaultId);
        if (!active) {
            state.convertToGroupModal = null;
        } else {
            const overlay = createElement('div', 'modal-overlay', []);
            const modal = createElement('div', 'modal modal-convert-group', []);
            const title = createElement('div', 'modal-title', ['In Gruppe konvertieren']);

            const prefixLabel = createElement('label', 'modal-label', ['Gruppen-Prefix (z.B. POSTGRES_)']);
            const prefixInput = createElement('input', 'input modal-input', []);
            prefixInput.placeholder = 'POSTGRES_';
            prefixInput.value = state.convertToGroupModal.prefix;
            prefixInput.oninput = (e) => {
                state.convertToGroupModal.prefix = e.target.value;
            };

            const listLabel = createElement('div', 'modal-label convert-group-list-label', ['Keys – Suffix anpassen']);
            const listWrap = createElement('div', 'convert-group-list', []);
            state.convertToGroupModal.items.forEach((item, i) => {
                const row = createElement('div', 'convert-group-list-row', []);
                const keyLabel = createElement('span', 'convert-group-key-label', [item.currentKey]);
                const suffixInput = createElement('input', 'input convert-group-suffix', []);
                suffixInput.value = item.suffix;
                suffixInput.oninput = (e) => {
                    state.convertToGroupModal.items[i].suffix = e.target.value;
                };
                row.appendChild(keyLabel);
                row.appendChild(suffixInput);
                listWrap.appendChild(row);
            });

            const actions = createElement('div', 'modal-actions', []);
            const cancelBtn = createElement('button', 'btn-secondary', ['Abbrechen']);
            cancelBtn.onclick = () => {
                state.convertToGroupModal = null;
                render();
            };
            const convertBtn = createElement('button', 'btn-primary', ['Konvertieren']);
            convertBtn.onclick = async () => {
                const prefix = (state.convertToGroupModal.prefix || '').trim();
                if (!prefix) return;
                const entries = [...(active.entries || [])];
                state.convertToGroupModal.items.forEach(({ index, suffix }) => {
                    const fullKey = prefix + (suffix || '').trim();
                    if (entries[index]) {
                        entries[index] = { ...entries[index], key: fullKey, groupPrefix: prefix };
                    }
                });
                try {
                    await UpdateVault({ ...active, entries });
                    const fresh = await GetVault(active.id);
                    state.vaults = state.vaults.map(v => v.id === fresh.id ? fresh : v);
                    state.selectedEntryIndices = new Set();
                    state.convertToGroupModal = null;
                    render();
                } catch (err) {
                    console.error('Failed to convert to group', err);
                }
            };

            actions.appendChild(cancelBtn);
            actions.appendChild(convertBtn);
            modal.appendChild(title);
            modal.appendChild(prefixLabel);
            modal.appendChild(prefixInput);
            modal.appendChild(listLabel);
            modal.appendChild(listWrap);
            modal.appendChild(actions);
            overlay.appendChild(modal);
            overlay.onclick = (e) => {
                if (e.target === overlay) {
                    state.convertToGroupModal = null;
                    render();
                }
            };
            app.appendChild(overlay);
        }
    }

    if (state.duplicateConfirmModal) {
        const { target, keysToOverwrite } = state.duplicateConfirmModal;
        const targetLabel = target === 'staging' ? 'STAGING' : 'PROD';
        const keyList = keysToOverwrite.length <= 3
            ? keysToOverwrite.join(', ')
            : keysToOverwrite.slice(0, 2).join(', ') + ' und ' + (keysToOverwrite.length - 2) + ' weitere';
        const overlay = createElement('div', 'modal-overlay', []);
        const modal = createElement('div', 'modal', []);
        const title = createElement('div', 'modal-title', ['Duplizieren bestätigen']);
        const text = createElement('p', 'duplicate-confirm-text', []);
        text.textContent = `${targetLabel} hat für die ausgewählten Keys bereits Einträge (${keyList}). Willst du sie wirklich duplizieren? Hinweis: Die Werte in ${targetLabel} werden dabei überschrieben!`;
        const actions = createElement('div', 'modal-actions', []);
        const cancelBtn = createElement('button', 'btn-secondary', ['Abbrechen']);
        cancelBtn.onclick = () => {
            state.duplicateConfirmModal = null;
            render();
        };
        const confirmBtn = createElement('button', 'btn-primary', ['Duplizieren']);
        confirmBtn.onclick = () => {
            duplicateDevToTarget(target);
        };
        actions.appendChild(cancelBtn);
        actions.appendChild(confirmBtn);
        modal.appendChild(title);
        modal.appendChild(text);
        modal.appendChild(actions);
        overlay.appendChild(modal);
        overlay.onclick = (e) => {
            if (e.target === overlay) {
                state.duplicateConfirmModal = null;
                render();
            }
        };
        app.appendChild(overlay);
    }
}

document.addEventListener('click', hideContextMenu);
document.addEventListener('contextmenu', (e) => {
    // Global Browser-Kontextmenü deaktivieren, eigenes Menü wird separat gerendert.
    e.preventDefault();
});

loadVaults();
