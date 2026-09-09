import './style.css';
import './app.css';

import {
    ListVaults,
    CreateVault,
    GetVault,
    UpdateVault,
    DeleteVault,
    ChooseVaultIcon,
    StartupError,
    ChooseEnvFile,
    AnalyseEnvImport,
    AnalyseEnvForVault,
    ApplyEnvValues,
    CreateVaultFromImport,
    ChooseBackupTarget,
    ChooseBackupSource,
    BackupVaults,
    RestoreVaults,
    MinBackupPassphraseLength,
} from '../wailsjs/go/main/App';

const app = document.querySelector('#app');

/* ==========================================================================
   Konstanten
   ========================================================================== */

const ENVS = ['dev', 'staging', 'prod'];

const ENV_LABEL = { dev: 'DEV', staging: 'STAGING', prod: 'PROD' };

// Feldname im Go-Modell je Umgebung.
const ENV_FIELD = { dev: 'valueDev', staging: 'valueStage', prod: 'valueProd' };

const TYPES = ['secret', 'env', 'note'];

/* ==========================================================================
   Icons (Inline-SVG, Feather-Stil)
   ========================================================================== */

function icon(name, size = 16) {
    const paths = {
        search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>',
        x: '<path d="M18 6L6 18M6 6l12 12"/>',
        plus: '<path d="M12 5v14M5 12h14"/>',
        copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
        edit: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
        eye: '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>',
        eyeOff: '<path d="M17.9 17.9A10.1 10.1 0 0 1 12 20c-7 0-11-8-11-8a18.5 18.5 0 0 1 5.1-5.9M9.9 4.2A9.1 9.1 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.2 3.2m-6.7-1.1a3 3 0 1 1-4.2-4.2"/><path d="M1 1l22 22"/>',
        trash: '<path d="M3 6h18"/><path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
        chevronDown: '<path d="M6 9l6 6 6-6"/>',
        more: '<circle cx="12" cy="5" r="1.6"/><circle cx="12" cy="12" r="1.6"/><circle cx="12" cy="19" r="1.6"/>',
        check: '<path d="M20 6L9 17l-5-5"/>',
        vault: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="12" cy="12" r="3.2"/><path d="M12 8.8V7M12 17v-1.8M15.2 12H17M7 12h1.8"/>',
        key: '<circle cx="7.5" cy="15.5" r="4.5"/><path d="M10.8 12.2L21 2m-4 4l3 3m-6-6l3 3"/>',
        alert: '<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 16.5v.01"/>',
        arrowRight: '<path d="M5 12h14M13 6l6 6-6 6"/>',
        layers: '<path d="M12 2l9 5-9 5-9-5 9-5z"/><path d="M3 12l9 5 9-5"/><path d="M3 17l9 5 9-5"/>',
        archive: '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/>',
        file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
        upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 9l5-5 5 5"/><path d="M12 4v12"/>',
        download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 11l5 5 5-5"/><path d="M12 16V4"/>',
    };

    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', String(size));
    svg.setAttribute('height', String(size));
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    // Nur statische, im Code definierte Pfade – nie Benutzerdaten.
    svg.innerHTML = paths[name] || '';
    return svg;
}

/* ==========================================================================
   DOM-Helfer
   ========================================================================== */

function el(tag, className, children) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    for (const child of children || []) {
        if (child === null || child === undefined || child === false) continue;
        node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
    }
    return node;
}

function button(className, children, onClick, title) {
    const b = el('button', className, children);
    b.type = 'button';
    if (title) b.title = title;
    if (onClick) b.onclick = onClick;
    return b;
}

function iconButton(className, iconName, onClick, title, size = 15) {
    return button(className, [icon(iconName, size)], onClick, title);
}

/* ==========================================================================
   State
   ========================================================================== */

const state = {
    vaults: [],
    activeVaultId: null,
    startupError: '',

    // '' = alle drei Umgebungen nebeneinander, sonst genau eine.
    envFilter: '',
    search: '',
    revealAll: false,
    revealedRows: new Set(),

    collapsedGroups: {},
    selected: new Set(),

    // { index, field } mit field aus 'key' | 'dev' | 'staging' | 'prod' | 'type'
    editing: null,

    addPanel: null,
    vaultModal: null,
    importReview: null,
    envImport: null,
    backupModal: null,
    convertModal: null,
    duplicateModal: null,
    confirmModal: null,
    contextMenu: null,

    toasts: [],
};

let toastSeq = 0;

/* ==========================================================================
   Feedback
   ========================================================================== */

function toast(message, kind = 'info') {
    const id = ++toastSeq;
    state.toasts.push({ id, message, kind });
    render();
    setTimeout(() => {
        state.toasts = state.toasts.filter(t => t.id !== id);
        render();
    }, kind === 'error' ? 6000 : 2200);
}

function describeError(err) {
    if (!err) return 'Unbekannter Fehler';
    if (typeof err === 'string') return err;
    return err.message || String(err);
}

/* ==========================================================================
   Datenzugriff
   ========================================================================== */

function activeVault() {
    return state.vaults.find(v => v.id === state.activeVaultId) || null;
}

async function loadVaults() {
    try {
        state.startupError = await StartupError();
    } catch (err) {
        console.error('StartupError konnte nicht gelesen werden', err);
        state.startupError = '';
    }

    try {
        state.vaults = await ListVaults();
        if (state.activeVaultId && !state.vaults.some(v => v.id === state.activeVaultId)) {
            state.activeVaultId = null;
        }
        if (!state.activeVaultId && state.vaults.length > 0) {
            state.activeVaultId = state.vaults[0].id;
        }
    } catch (err) {
        console.error('Vaults konnten nicht geladen werden', err);
        toast('Vaults konnten nicht geladen werden: ' + describeError(err), 'error');
    }
    render();
}

// Zentraler Schreibpfad: nimmt die aktuellen Entries, laesst den Aufrufer sie
// veraendern und schreibt das Ergebnis zurueck. Jeder Fehler wird sichtbar –
// vorher landeten Speicherfehler nur in der Konsole.
async function mutateEntries(mutator, successMessage) {
    const vault = activeVault();
    if (!vault) return false;

    const entries = (vault.entries || []).map(e => ({ ...e }));
    const next = mutator(entries);
    if (next === false) return false;

    try {
        await UpdateVault({ ...vault, entries: Array.isArray(next) ? next : entries });
        const fresh = await GetVault(vault.id);
        state.vaults = state.vaults.map(v => (v.id === fresh.id ? fresh : v));
        if (successMessage) toast(successMessage, 'success');
        return true;
    } catch (err) {
        console.error('Speichern fehlgeschlagen', err);
        toast('Speichern fehlgeschlagen: ' + describeError(err), 'error');
        return false;
    }
}

async function copyValue(text, fieldNode) {
    if (!text) {
        toast('Nichts zu kopieren – der Wert ist leer.');
        return;
    }
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(text);
        } else {
            const tmp = document.createElement('textarea');
            tmp.value = text;
            document.body.appendChild(tmp);
            tmp.select();
            document.execCommand('copy');
            document.body.removeChild(tmp);
        }
        if (fieldNode) {
            fieldNode.classList.add('is-copied');
            setTimeout(() => fieldNode.classList.remove('is-copied'), 700);
        }
        toast('Kopiert', 'success');
    } catch (err) {
        console.error('Kopieren fehlgeschlagen', err);
        toast('Kopieren fehlgeschlagen', 'error');
    }
}

/* ==========================================================================
   Ableitungen
   ========================================================================== */

function visibleEnvs() {
    return state.envFilter ? [state.envFilter] : ENVS;
}

function entryMatchesSearch(entry) {
    const q = state.search.trim().toLowerCase();
    if (!q) return true;
    if ((entry.key || '').toLowerCase().includes(q)) return true;
    // Werte werden mitdurchsucht, damit man eine bekannte URL wiederfindet.
    return ENVS.some(env => (entry[ENV_FIELD[env]] || '').toLowerCase().includes(q));
}

// Baut die Anzeigestruktur: Gruppen in der Reihenfolge ihres ersten Auftretens,
// ungruppierte Keys immer als letzter Block.
function buildGroups(entries) {
    const map = new Map();

    entries.forEach((entry, index) => {
        if (!entryMatchesSearch(entry)) return;
        const prefix = entry.groupPrefix || '';
        if (!map.has(prefix)) {
            map.set(prefix, { key: prefix, prefix, isUngrouped: prefix === '', items: [] });
        }
        const fullKey = entry.key || '';
        const suffix = prefix && fullKey.startsWith(prefix) ? fullKey.slice(prefix.length) : fullKey;
        map.get(prefix).items.push({ entry, index, suffix });
    });

    const groups = [...map.values()];
    return [
        ...groups.filter(g => !g.isUngrouped),
        ...groups.filter(g => g.isUngrouped),
    ];
}

function isGroupCollapsed(groupKey) {
    // Standard ist aufgeklappt. Frueher war es umgekehrt, wodurch ein frisch
    // geoeffneter Vault praktisch leer aussah.
    return state.collapsedGroups[groupKey] === true;
}

function maskFor(value) {
    if (!value) return '';
    return '•'.repeat(Math.min(Math.max(value.length, 6), 18));
}

function clearSelection() {
    state.selected = new Set();
}

/* ==========================================================================
   Topbar
   ========================================================================== */

function renderTopbar() {
    const brand = el('div', 'topbar-brand', [
        el('div', 'topbar-logo', ['T']),
        'TForge',
    ]);

    const searchInput = el('input', 'search-input');
    searchInput.type = 'text';
    searchInput.placeholder = 'Keys und Werte durchsuchen …';
    searchInput.value = state.search;
    searchInput.oninput = (e) => {
        state.search = e.target.value;
        renderMainOnly();
    };
    searchInput.dataset.focusKey = 'search';

    const searchBox = el('div', 'search-box', [
        el('span', 'search-icon', [icon('search', 15)]),
        searchInput,
        state.search
            ? iconButton('search-clear', 'x', () => {
                state.search = '';
                render();
            }, 'Suche zurücksetzen', 14)
            : null,
    ]);

    const revealBtn = button(
        'btn ' + (state.revealAll ? 'btn-primary' : 'btn-ghost'),
        [icon(state.revealAll ? 'eyeOff' : 'eye', 15), state.revealAll ? 'Verbergen' : 'Aufdecken'],
        () => {
            state.revealAll = !state.revealAll;
            state.revealedRows = new Set();
            render();
        },
        'Alle Secret-Werte im Klartext anzeigen'
    );

    const backupBtn = button(
        'btn btn-ghost',
        [icon('archive', 15), 'Backup'],
        openBackupModal,
        'Vaults sichern oder aus einem Backup wiederherstellen'
    );

    return el('div', 'topbar', [brand, searchBox, el('div', 'toolbar-spacer'), revealBtn, backupBtn]);
}

/* ==========================================================================
   Sidebar
   ========================================================================== */

function vaultAvatar(vault, className) {
    const letter = (vault.name || '').trim().charAt(0).toUpperCase() || 'V';
    if (vault.icon && vault.icon.trim()) {
        const img = document.createElement('img');
        img.className = className;
        img.src = vault.icon;
        img.alt = '';
        img.onerror = () => img.replaceWith(el('div', className, [letter]));
        return img;
    }
    return el('div', className, [letter]);
}

function renderSidebar() {
    const list = el('div', 'vault-list');

    if (state.vaults.length === 0) {
        list.appendChild(el('div', 'vault-empty', [
            'Noch keine Vaults. Lege unten einen an, um Keys zu verwalten.',
        ]));
    } else {
        for (const vault of state.vaults) {
            const count = (vault.entries || []).length;
            const item = el('div', 'vault-item' + (vault.id === state.activeVaultId ? ' is-active' : ''), [
                vaultAvatar(vault, 'vault-avatar'),
                el('div', 'vault-item-main', [
                    el('div', 'vault-item-name', [vault.name || 'Ohne Namen']),
                    el('div', 'vault-item-meta', [
                        vault.description ? vault.description : `${count} ${count === 1 ? 'Key' : 'Keys'}`,
                    ]),
                ]),
                iconButton('btn-icon vault-item-menu', 'more', (e) => {
                    e.stopPropagation();
                    const rect = e.currentTarget.getBoundingClientRect();
                    openVaultMenu(vault.id, rect.right - 4, rect.bottom + 4);
                }, 'Aktionen', 15),
            ]);

            item.onclick = () => {
                if (state.activeVaultId === vault.id) return;
                state.activeVaultId = vault.id;
                clearSelection();
                state.editing = null;
                state.addPanel = null;
                state.revealedRows = new Set();
                render();
            };

            item.oncontextmenu = (e) => {
                e.preventDefault();
                e.stopPropagation();
                openVaultMenu(vault.id, e.clientX, e.clientY);
            };

            list.appendChild(item);
        }
    }

    return el('div', 'sidebar', [
        el('div', 'sidebar-head', [
            el('span', 'sidebar-title', ['Vaults']),
            el('span', 'sidebar-count', [String(state.vaults.length)]),
        ]),
        list,
        el('div', 'sidebar-foot', [
            button('btn btn-ghost', [icon('plus', 15), 'Neuer Vault'], openCreateVault),
        ]),
    ]);
}

function openVaultMenu(vaultId, x, y) {
    state.contextMenu = {
        x,
        y,
        items: [
            {
                label: 'Bearbeiten',
                iconName: 'edit',
                action: () => openEditVault(vaultId),
            },
            { separator: true },
            {
                label: 'Vault löschen',
                iconName: 'trash',
                danger: true,
                action: () => requestDeleteVault(vaultId),
            },
        ],
    };
    render();
}

/* ==========================================================================
   Vault-Aktionen
   ========================================================================== */

function openCreateVault() {
    state.vaultModal = {
        mode: 'create',
        id: null,
        name: '',
        description: '',
        icon: '',
        // 'blank' legt einen leeren Vault an, 'import' liest .env-Dateien ein.
        source: 'blank',
        files: { example: null, dev: null, staging: null, prod: null },
        busy: false,
    };
    render();
}

function openEditVault(id) {
    const vault = state.vaults.find(v => v.id === id);
    if (!vault) return;
    state.vaultModal = {
        mode: 'edit',
        id,
        name: vault.name || '',
        description: vault.description || '',
        icon: vault.icon || '',
    };
    state.contextMenu = null;
    render();
}

function requestDeleteVault(id) {
    const vault = state.vaults.find(v => v.id === id);
    if (!vault) return;
    const count = (vault.entries || []).length;
    state.contextMenu = null;
    state.confirmModal = {
        title: 'Vault löschen?',
        description: `„${vault.name}“ enthält ${count} ${count === 1 ? 'Key' : 'Keys'}. `
            + 'Das Löschen kann nicht rückgängig gemacht werden.',
        confirmLabel: 'Endgültig löschen',
        action: async () => {
            try {
                await DeleteVault(id);
                state.vaults = state.vaults.filter(v => v.id !== id);
                if (state.activeVaultId === id) {
                    state.activeVaultId = state.vaults.length > 0 ? state.vaults[0].id : null;
                }
                clearSelection();
                toast('Vault gelöscht', 'success');
            } catch (err) {
                console.error('Löschen fehlgeschlagen', err);
                toast('Löschen fehlgeschlagen: ' + describeError(err), 'error');
            }
        },
    };
    render();
}

/* ==========================================================================
   Hauptbereich
   ========================================================================== */

function renderMain() {
    const vault = activeVault();

    if (!vault) {
        return el('div', 'main', [
            el('div', 'empty', [
                el('div', 'empty-inner', [
                    el('div', 'empty-icon', [icon('vault', 44)]),
                    el('div', 'empty-title', ['Kein Vault ausgewählt']),
                    el('div', 'empty-text', [
                        state.vaults.length === 0
                            ? 'Ein Vault bündelt die Keys eines Projekts – etwa Datenbank-Zugang, API-Schlüssel und Feature-Flags, jeweils mit eigenen Werten für DEV, STAGING und PROD.'
                            : 'Wähle links einen Vault aus, um seine Keys zu sehen.',
                    ]),
                    state.vaults.length === 0
                        ? button('btn btn-primary', [icon('plus', 15), 'Ersten Vault anlegen'], openCreateVault)
                        : null,
                ]),
            ]),
        ]);
    }

    const entries = vault.entries || [];
    const groups = buildGroups(entries);
    const shownCount = groups.reduce((sum, g) => sum + g.items.length, 0);
    const groupCount = groups.filter(g => !g.isUngrouped).length;

    const main = el('div', 'main', []);
    main.appendChild(renderMainHead(vault, entries.length, shownCount, groupCount));
    main.appendChild(renderToolbar());

    const scroll = el('div', 'table-scroll' + (state.selected.size > 0 ? ' has-selection' : ''), []);
    scroll.dataset.scrollKey = 'table';

    if (entries.length === 0) {
        scroll.appendChild(el('div', 'empty', [
            el('div', 'empty-inner', [
                el('div', 'empty-icon', [icon('key', 40)]),
                el('div', 'empty-title', ['Noch keine Keys']),
                el('div', 'empty-text', [
                    'Lege einzelne Keys an oder gleich eine ganze Gruppe mit gemeinsamem Prefix, etwa POSTGRES_ mit HOST, PORT und PASSWORD.',
                ]),
                button('btn btn-primary', [icon('plus', 15), 'Key hinzufügen'], () => openAddPanel('single')),
            ]),
        ]));
    } else if (shownCount === 0) {
        scroll.appendChild(el('div', 'empty', [
            el('div', 'empty-inner', [
                el('div', 'empty-icon', [icon('search', 40)]),
                el('div', 'empty-title', ['Keine Treffer']),
                el('div', 'empty-text', [`Kein Key oder Wert passt zu „${state.search}“.`]),
                button('btn btn-ghost', ['Suche zurücksetzen'], () => {
                    state.search = '';
                    render();
                }),
            ]),
        ]));
    } else {
        scroll.appendChild(renderKeyTable(groups));
    }

    if (state.addPanel) {
        scroll.appendChild(renderAddPanel(vault));
    }

    main.appendChild(scroll);
    return main;
}

function renderMainHead(vault, totalKeys, shownKeys, groupCount) {
    // Bei aktiver Suche beziehen sich beide Zahlen auf die Treffer, sonst
    // stuende hier die Gesamtzahl der Keys neben der Zahl gefilterter Gruppen.
    const filtering = state.search.trim() !== '';
    const parts = [
        filtering
            ? `${shownKeys} von ${totalKeys} Keys`
            : `${totalKeys} ${totalKeys === 1 ? 'Key' : 'Keys'}`,
    ];
    if (groupCount > 0) parts.push(`${groupCount} ${groupCount === 1 ? 'Gruppe' : 'Gruppen'}`);
    if (vault.description) parts.push(vault.description);

    return el('div', 'main-head', [
        el('div', 'main-title-row', [
            el('div', null, [
                el('div', 'main-title', [vault.name || 'Ohne Namen']),
                el('div', 'main-subtitle', [parts.join(' · ')]),
            ]),
            el('div', 'main-title-spacer'),
            button('btn btn-ghost btn-sm', [icon('edit', 14), 'Bearbeiten'], () => openEditVault(vault.id)),
        ]),
    ]);
}

function renderToolbar() {
    const filter = el('div', 'env-filter', []);

    const allTab = button('env-tab' + (state.envFilter === '' ? ' is-active' : ''), ['ALLE'], () => {
        state.envFilter = '';
        state.editing = null;
        render();
    }, 'Alle Umgebungen nebeneinander');
    filter.appendChild(allTab);

    for (const env of ENVS) {
        filter.appendChild(button(
            'env-tab' + (state.envFilter === env ? ' is-active' : ''),
            [el('span', `env-dot for-${env}`), ENV_LABEL[env]],
            () => {
                state.envFilter = env;
                state.editing = null;
                render();
            },
            `Nur ${ENV_LABEL[env]} anzeigen`
        ));
    }

    return el('div', 'main-toolbar', [
        filter,
        el('div', 'toolbar-spacer'),
        button('btn btn-ghost btn-sm', [icon('layers', 14), 'Gruppe'], () => openAddPanel('group')),
        button('btn btn-primary btn-sm', [icon('plus', 14), 'Key'], () => openAddPanel('single')),
    ]);
}

/* ==========================================================================
   Auswahl-Leiste – ersetzt das frühere versteckte Kontextmenü
   ========================================================================== */

function renderSelectionBar() {
    const n = state.selected.size;

    return el('div', 'selection-bar', [
        el('span', 'selection-count', [`${n} ausgewählt`]),
        el('span', 'selection-sep'),
        button('btn btn-ghost btn-sm', [icon('layers', 14), 'Zu Gruppe zusammenfassen'], openConvertModal),
        button('btn btn-ghost btn-sm', ['DEV', icon('arrowRight', 13), 'STAGING'], () => requestDuplicate('staging')),
        button('btn btn-ghost btn-sm', ['DEV', icon('arrowRight', 13), 'PROD'], () => requestDuplicate('prod')),
        el('span', 'selection-sep'),
        button('btn btn-danger btn-sm', [icon('trash', 14), 'Löschen'], requestDeleteSelected),
        iconButton('btn-icon', 'x', () => {
            clearSelection();
            render();
        }, 'Auswahl aufheben', 15),
    ]);
}

function requestDeleteSelected() {
    const vault = activeVault();
    if (!vault) return;
    const indices = [...state.selected];
    const keys = indices.map(i => (vault.entries[i] || {}).key || '').filter(Boolean);

    state.confirmModal = {
        title: `${indices.length} ${indices.length === 1 ? 'Key' : 'Keys'} löschen?`,
        description: 'Die Werte für alle drei Umgebungen gehen dabei verloren.',
        keys,
        confirmLabel: 'Löschen',
        action: async () => {
            const drop = new Set(indices);
            const ok = await mutateEntries(
                entries => entries.filter((_, i) => !drop.has(i)),
                `${indices.length} ${indices.length === 1 ? 'Key' : 'Keys'} gelöscht`
            );
            if (ok) clearSelection();
        },
    };
    render();
}

function requestDuplicate(target) {
    const vault = activeVault();
    if (!vault) return;

    const indices = [...state.selected];
    const field = ENV_FIELD[target];
    const overwrites = indices
        .map(i => vault.entries[i])
        .filter(e => e && (e[field] || '').trim() !== '')
        .map(e => e.key || '');

    if (overwrites.length > 0) {
        state.duplicateModal = { target, overwrites, count: indices.length };
        render();
        return;
    }
    applyDuplicate(target);
}

async function applyDuplicate(target) {
    const indices = new Set(state.selected);
    const field = ENV_FIELD[target];

    const ok = await mutateEntries(entries => {
        entries.forEach((entry, i) => {
            if (indices.has(i)) entry[field] = entry.valueDev || '';
        });
        return entries;
    }, `Nach ${ENV_LABEL[target]} übernommen`);

    state.duplicateModal = null;
    if (ok) clearSelection();
    render();
}

/* ==========================================================================
   Key-Tabelle
   ========================================================================== */

function renderKeyTable(groups) {
    const envs = visibleEnvs();
    const table = el('div', 'key-table', []);
    table.dataset.envs = String(envs.length);

    // Kopfzeile
    const head = el('div', 'key-row key-row-head', []);
    head.appendChild(el('div', null, ['']));
    head.appendChild(el('div', null, ['Key']));
    for (const env of envs) {
        // Farbe steckt im Text selbst; ein vorangestellter Punkt haette die
        // Beschriftung gegen die Werte darunter verschoben.
        head.appendChild(el('div', `col-head for-${env}`, [ENV_LABEL[env]]));
    }
    head.appendChild(el('div', null, ['Typ']));
    head.appendChild(el('div', null, ['']));
    table.appendChild(head);

    for (const group of groups) {
        if (!group.isUngrouped) {
            const collapsed = isGroupCollapsed(group.key);
            const header = el('div', 'group-row' + (collapsed ? ' is-collapsed' : ''), [
                el('span', 'group-caret', [icon('chevronDown', 14)]),
                el('span', 'group-name', [group.prefix.replace(/_$/, '')]),
                el('span', 'group-badge', [String(group.items.length)]),
            ]);
            header.onclick = () => {
                state.collapsedGroups[group.key] = !collapsed;
                render();
            };
            table.appendChild(header);
            if (collapsed) continue;
        }

        for (const item of group.items) {
            table.appendChild(renderEntryRow(item, group, envs));
        }
    }

    return table;
}

function renderEntryRow(item, group, envs) {
    const { entry, index, suffix } = item;
    const selected = state.selected.has(index);

    const row = el('div', 'key-row key-row-entry' + (selected ? ' is-selected' : ''), []);

    row.oncontextmenu = (e) => {
        e.preventDefault();
        e.stopPropagation();
        if (!state.selected.has(index)) {
            state.selected = new Set([index]);
        }
        openRowMenu(e.clientX, e.clientY);
    };

    // Auswahl
    const check = button('row-check' + (selected ? ' is-checked' : ''), [icon('check', 11)], (e) => {
        e.stopPropagation();
        toggleSelected(index, e.shiftKey);
        render();
    }, 'Auswählen');
    row.appendChild(el('div', 'cell cell-check', [check]));

    // Key
    row.appendChild(el('div', 'cell cell-key', [renderKeyField(entry, index, group, suffix)]));

    // Werte je Umgebung
    for (const env of envs) {
        const cell = el('div', 'cell cell-value', [renderValueField(entry, index, env)]);
        cell.dataset.env = env;
        row.appendChild(cell);
    }

    // Typ
    row.appendChild(el('div', 'cell cell-type', [renderTypeField(entry, index)]));

    // Zeile löschen
    row.appendChild(el('div', 'cell cell-actions', [
        iconButton('btn-icon is-danger', 'trash', () => {
            state.confirmModal = {
                title: 'Key löschen?',
                description: `„${entry.key}“ wird mit allen Werten entfernt.`,
                confirmLabel: 'Löschen',
                action: () => mutateEntries(
                    entries => entries.filter((_, i) => i !== index),
                    'Key gelöscht'
                ),
            };
            render();
        }, 'Key löschen', 14),
    ]));

    return row;
}

function toggleSelected(index, additive) {
    if (!additive) {
        if (state.selected.has(index)) state.selected.delete(index);
        else state.selected.add(index);
        return;
    }
    // Shift: Bereich vom zuletzt gewaehlten Index bis hier.
    const existing = [...state.selected];
    if (existing.length === 0) {
        state.selected.add(index);
        return;
    }
    const last = existing[existing.length - 1];
    const [from, to] = last < index ? [last, index] : [index, last];
    for (let i = from; i <= to; i++) state.selected.add(i);
}

/* --- Key-Zelle --------------------------------------------------------- */

function renderKeyField(entry, index, group, suffix) {
    const grouped = !group.isUngrouped && group.prefix;
    const editing = state.editing && state.editing.index === index && state.editing.field === 'key';

    if (editing) {
        const input = el('input', 'input');
        input.type = 'text';
        input.value = grouped ? suffix : (entry.key || '');
        input.dataset.focusKey = `key-${index}`;
        input.onkeydown = (e) => {
            if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
            if (e.key === 'Escape') { e.preventDefault(); state.editing = null; render(); }
        };
        input.onblur = () => {
            const raw = input.value.trim();
            const nextKey = grouped ? group.prefix + raw : raw;
            state.editing = null;
            if (!raw || nextKey === entry.key) { render(); return; }
            mutateEntries(entries => {
                entries[index] = { ...entries[index], key: nextKey };
                return entries;
            }).then(render);
        };
        return input;
    }

    const shown = grouped ? suffix : (entry.key || '');
    const fullKey = entry.key || '';

    const field = el('div', 'field', []);
    field.appendChild(el('span', 'field-text is-key' + (shown ? '' : ' is-empty'), [shown || 'ohne Namen']));
    field.appendChild(el('div', 'field-actions', [
        iconButton('field-btn', 'copy', (e) => {
            e.stopPropagation();
            copyValue(fullKey, field);
        }, 'Key kopieren', 13),
    ]));
    field.title = fullKey;
    field.onclick = (e) => {
        if (e.ctrlKey || e.metaKey) {
            toggleSelected(index, false);
            render();
            return;
        }
        state.editing = { index, field: 'key' };
        render();
    };
    return field;
}

/* --- Wert-Zelle -------------------------------------------------------- */

function renderValueField(entry, index, env) {
    const editing = state.editing && state.editing.index === index && state.editing.field === env;
    const value = entry[ENV_FIELD[env]] || '';

    if (editing) {
        const input = el('input', `input for-${env}`);
        input.type = 'text';
        input.value = value;
        input.dataset.focusKey = `${env}-${index}`;
        input.onkeydown = (e) => {
            if (e.key === 'Enter') { e.preventDefault(); input.blur(); }
            if (e.key === 'Escape') { e.preventDefault(); state.editing = null; render(); }
            if (e.key === 'Tab') {
                // Tab springt zur naechsten Umgebung derselben Zeile.
                const envs = visibleEnvs();
                const pos = envs.indexOf(env);
                const nextEnv = envs[e.shiftKey ? pos - 1 : pos + 1];
                if (nextEnv) {
                    e.preventDefault();
                    input.dataset.moveTo = nextEnv;
                    input.blur();
                }
            }
        };
        input.onblur = () => {
            const nextValue = input.value;
            const moveTo = input.dataset.moveTo;
            state.editing = moveTo ? { index, field: moveTo } : null;
            if (nextValue === value) { render(); return; }
            mutateEntries(entries => {
                entries[index] = { ...entries[index], [ENV_FIELD[env]]: nextValue };
                return entries;
            }).then(render);
        };
        return input;
    }

    const isSecret = entry.type === 'secret';
    const revealed = state.revealAll || state.revealedRows.has(index);
    const display = isSecret && !revealed ? maskFor(value) : value;

    const field = el('div', 'field', []);
    field.appendChild(el(
        'span',
        'field-text' + (value ? (isSecret && !revealed ? ' is-masked' : '') : ' is-empty'),
        [value ? display : 'leer']
    ));

    const actions = el('div', 'field-actions', []);
    if (isSecret && value) {
        actions.appendChild(iconButton('field-btn', revealed ? 'eyeOff' : 'eye', (e) => {
            e.stopPropagation();
            if (state.revealedRows.has(index)) state.revealedRows.delete(index);
            else state.revealedRows.add(index);
            render();
        }, revealed ? 'Verbergen' : 'Anzeigen', 13));
    }
    if (value) {
        actions.appendChild(iconButton('field-btn', 'copy', (e) => {
            e.stopPropagation();
            copyValue(value, field);
        }, 'Wert kopieren', 13));
    }
    field.appendChild(actions);

    field.onclick = (e) => {
        if (e.ctrlKey || e.metaKey) {
            toggleSelected(index, false);
            render();
            return;
        }
        state.editing = { index, field: env };
        render();
    };
    return field;
}

/* --- Typ-Zelle --------------------------------------------------------- */

function renderTypeField(entry, index) {
    const editing = state.editing && state.editing.index === index && state.editing.field === 'type';
    const type = entry.type || 'env';

    if (editing) {
        const select = el('select', 'select');
        select.dataset.focusKey = `type-${index}`;
        for (const t of TYPES) {
            const opt = el('option', null, [t]);
            opt.value = t;
            if (t === type) opt.selected = true;
            select.appendChild(opt);
        }
        select.onchange = () => {
            const next = select.value;
            state.editing = null;
            mutateEntries(entries => {
                entries[index] = { ...entries[index], type: next };
                return entries;
            }).then(render);
        };
        select.onblur = () => {
            state.editing = null;
            render();
        };
        return select;
    }

    return button(`type-badge for-${type}`, [type], (e) => {
        e.stopPropagation();
        state.editing = { index, field: 'type' };
        render();
    }, 'Typ ändern');
}

/* --- Zeilen-Kontextmenü (Beschleuniger, nicht der einzige Weg) --------- */

function openRowMenu(x, y) {
    const n = state.selected.size;
    state.contextMenu = {
        x,
        y,
        items: [
            { label: 'Zu Gruppe zusammenfassen', iconName: 'layers', action: openConvertModal },
            { label: 'DEV nach STAGING kopieren', iconName: 'arrowRight', action: () => requestDuplicate('staging') },
            { label: 'DEV nach PROD kopieren', iconName: 'arrowRight', action: () => requestDuplicate('prod') },
            { separator: true },
            {
                label: `${n} ${n === 1 ? 'Key' : 'Keys'} löschen`,
                iconName: 'trash',
                danger: true,
                action: requestDeleteSelected,
            },
        ],
    };
    render();
}

/* ==========================================================================
   Panel: neue Keys anlegen
   ========================================================================== */

function openAddPanel(mode) {
    const vault = activeVault();
    if (!vault) return;

    const prefixes = existingPrefixes(vault);
    state.addPanel = {
        mode,
        prefix: mode === 'group' ? (prefixes[0] || '') : '',
        customPrefix: prefixes.length === 0,
        keys: [''],
        focusIndex: 0,
    };
    render();
}

function existingPrefixes(vault) {
    const seen = [];
    for (const e of vault.entries || []) {
        if (e.groupPrefix && !seen.includes(e.groupPrefix)) seen.push(e.groupPrefix);
    }
    return seen;
}

function renderAddPanel(vault) {
    const panel = state.addPanel;
    const prefixes = existingPrefixes(vault);
    const wrap = el('div', 'add-panel', []);

    // Modus-Umschalter
    const segmented = el('div', 'segmented', [
        button('segmented-btn' + (panel.mode === 'single' ? ' is-active' : ''), ['Einzelne Keys'], () => {
            state.addPanel = { ...panel, mode: 'single' };
            render();
        }),
        button('segmented-btn' + (panel.mode === 'group' ? ' is-active' : ''), ['Gruppe'], () => {
            state.addPanel = { ...panel, mode: 'group', prefix: panel.prefix || prefixes[0] || '', customPrefix: prefixes.length === 0 };
            render();
        }),
    ]);

    wrap.appendChild(el('div', 'add-panel-head', [
        segmented,
        el('div', 'toolbar-spacer'),
        iconButton('btn-icon', 'x', () => {
            state.addPanel = null;
            render();
        }, 'Schließen', 15),
    ]));

    const body = el('div', 'add-panel-body', []);

    // Gruppen-Prefix
    if (panel.mode === 'group') {
        const prefixWrap = el('div', null, [el('div', 'add-field-label', ['Gruppen-Prefix'])]);

        if (prefixes.length > 0 && !panel.customPrefix) {
            const row = el('div', 'add-key-row', []);
            const select = el('select', 'select');
            select.style.height = '32px';
            select.style.flex = '1';
            for (const p of prefixes) {
                const opt = el('option', null, [p]);
                opt.value = p;
                if (p === panel.prefix) opt.selected = true;
                select.appendChild(opt);
            }
            const customOpt = el('option', null, ['Neues Prefix …']);
            customOpt.value = '';
            select.appendChild(customOpt);
            const customIndex = select.options.length - 1;
            select.onchange = () => {
                if (select.selectedIndex === customIndex) {
                    state.addPanel = { ...panel, customPrefix: true, prefix: '' };
                } else {
                    state.addPanel = { ...panel, prefix: select.value };
                }
                render();
            };
            row.appendChild(select);
            prefixWrap.appendChild(row);
        } else {
            const input = el('input', 'add-input');
            input.type = 'text';
            input.placeholder = 'z. B. POSTGRES_';
            input.value = panel.prefix;
            input.dataset.focusKey = 'add-prefix';
            input.oninput = (e) => { panel.prefix = e.target.value; };
            prefixWrap.appendChild(input);
        }
        body.appendChild(prefixWrap);
    }

    // Key-Zeilen
    const keysWrap = el('div', null, [
        el('div', 'add-field-label', [panel.mode === 'group' ? 'Keys in der Gruppe' : 'Keys']),
    ]);

    panel.keys.forEach((value, i) => {
        const row = el('div', 'add-key-row', []);
        if (panel.mode === 'group' && panel.prefix) {
            row.appendChild(el('div', 'add-prefix-tag', [panel.prefix]));
        }

        const input = el('input', 'add-input');
        input.type = 'text';
        input.placeholder = panel.mode === 'group' ? 'HOST' : 'NEXT_PUBLIC_API_URL';
        input.value = value;
        input.dataset.focusKey = `add-key-${i}`;
        input.oninput = (e) => { panel.keys[i] = e.target.value; };
        input.onkeydown = (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                if (e.shiftKey || i === panel.keys.length - 1) {
                    panel.keys.push('');
                    panel.focusIndex = panel.keys.length - 1;
                    render();
                } else {
                    submitAddPanel();
                }
            }
        };
        row.appendChild(input);

        if (panel.keys.length > 1) {
            row.appendChild(iconButton('btn-icon', 'x', () => {
                panel.keys.splice(i, 1);
                render();
            }, 'Zeile entfernen', 14));
        }
        keysWrap.appendChild(row);
    });

    keysWrap.appendChild(button('btn btn-ghost btn-sm', [icon('plus', 13), 'Weitere Zeile'], () => {
        panel.keys.push('');
        panel.focusIndex = panel.keys.length - 1;
        render();
    }));

    body.appendChild(keysWrap);
    wrap.appendChild(body);

    wrap.appendChild(el('div', 'add-panel-foot', [
        button('btn btn-primary btn-sm', ['Anlegen'], submitAddPanel),
        button('btn btn-ghost btn-sm', ['Abbrechen'], () => {
            state.addPanel = null;
            render();
        }),
        el('span', 'add-hint', ['Enter fügt eine weitere Zeile hinzu']),
    ]));

    return wrap;
}

async function submitAddPanel() {
    const panel = state.addPanel;
    if (!panel) return;

    const prefix = panel.mode === 'group' ? panel.prefix.trim() : '';
    if (panel.mode === 'group' && !prefix) {
        toast('Bitte ein Gruppen-Prefix angeben.', 'error');
        return;
    }

    const names = panel.keys.map(k => k.trim()).filter(Boolean);
    if (names.length === 0) {
        toast('Bitte mindestens einen Key angeben.', 'error');
        return;
    }

    const vault = activeVault();
    const existingKeys = new Set((vault.entries || []).map(e => e.key));
    const additions = [];
    const duplicates = [];

    for (const name of names) {
        const fullKey = prefix + name;
        if (existingKeys.has(fullKey)) {
            duplicates.push(fullKey);
            continue;
        }
        existingKeys.add(fullKey);
        additions.push({
            key: fullKey,
            valueDev: '',
            valueStage: '',
            valueProd: '',
            type: 'secret',
            groupPrefix: prefix,
        });
    }

    if (additions.length === 0) {
        toast(`Bereits vorhanden: ${duplicates.join(', ')}`, 'error');
        return;
    }

    const ok = await mutateEntries(
        entries => [...entries, ...additions],
        `${additions.length} ${additions.length === 1 ? 'Key' : 'Keys'} angelegt`
    );

    if (ok) {
        if (duplicates.length > 0) {
            toast(`Übersprungen, weil schon vorhanden: ${duplicates.join(', ')}`, 'error');
        }
        state.addPanel = null;
    }
    render();
}

/* ==========================================================================
   Modals
   ========================================================================== */

function modalShell(className, children, onClose) {
    const modal = el('div', 'modal ' + (className || ''), children);
    const overlay = el('div', 'modal-overlay', [modal]);
    overlay.onclick = (e) => {
        if (e.target === overlay) onClose();
    };
    return overlay;
}

function renderVaultModal() {
    const m = state.vaultModal;
    const close = () => { state.vaultModal = null; render(); };

    const nameInput = el('input', 'modal-input');
    nameInput.type = 'text';
    nameInput.value = m.name;
    nameInput.placeholder = 'z. B. Shop-Backend';
    nameInput.dataset.focusKey = 'vault-name';
    nameInput.oninput = (e) => { m.name = e.target.value; };

    const descInput = el('textarea', 'modal-textarea');
    descInput.value = m.description;
    descInput.placeholder = 'Wofür ist dieser Vault?';
    descInput.oninput = (e) => { m.description = e.target.value; };

    const preview = m.icon
        ? (() => {
            const img = document.createElement('img');
            img.className = 'modal-icon-preview';
            img.src = m.icon;
            img.alt = '';
            return img;
        })()
        : el('div', 'modal-icon-preview', [(m.name || '').trim().charAt(0).toUpperCase() || 'V']);

    const save = async () => {
        const name = (m.name || '').trim();
        if (!name) {
            toast('Bitte einen Namen angeben.', 'error');
            return;
        }
        try {
            if (m.mode === 'create') {
                const created = await CreateVault(name, m.description || '');
                if (m.icon) {
                    created.icon = m.icon;
                    await UpdateVault(created);
                }
                const fresh = await GetVault(created.id);
                state.vaults.push(fresh);
                state.vaults.sort((a, b) => (a.name || '').localeCompare(b.name || ''));
                state.activeVaultId = fresh.id;
                toast('Vault angelegt', 'success');
            } else {
                const existing = state.vaults.find(v => v.id === m.id);
                if (!existing) { close(); return; }
                await UpdateVault({ ...existing, name, description: m.description || '', icon: m.icon || '' });
                const fresh = await GetVault(m.id);
                state.vaults = state.vaults.map(v => (v.id === fresh.id ? fresh : v));
                toast('Gespeichert', 'success');
            }
            close();
        } catch (err) {
            console.error('Vault speichern fehlgeschlagen', err);
            toast('Speichern fehlgeschlagen: ' + describeError(err), 'error');
        }
    };

    const importing = m.mode === 'create' && m.source === 'import';

    const sourceToggle = m.mode !== 'create' ? null : el('div', 'segmented modal-source-toggle', [
        button('segmented-btn' + (m.source === 'blank' ? ' is-active' : ''), ['Leer'], () => {
            m.source = 'blank';
            render();
        }),
        button('segmented-btn' + (importing ? ' is-active' : ''), ['Aus .env-Dateien'], () => {
            m.source = 'import';
            render();
        }),
    ]);

    return modalShell(importing ? 'modal-wide' : '', [
        el('div', 'modal-title', [m.mode === 'create' ? 'Neuer Vault' : 'Vault bearbeiten']),
        el('div', 'modal-desc', [
            importing
                ? 'Die .env.example gibt die Struktur vor. Keys mit gemeinsamem Prefix werden automatisch gruppiert.'
                : 'Ein Vault bündelt die Keys eines Projekts.',
        ]),
        sourceToggle,
        el('div', 'modal-field', [el('label', 'modal-label', ['Name']), nameInput]),
        el('div', 'modal-field', [el('label', 'modal-label', ['Beschreibung']), descInput]),
        el('div', 'modal-field', [
            el('label', 'modal-label', ['Icon']),
            el('div', 'modal-icon-row', [
                preview,
                button('btn btn-ghost btn-sm', ['Bild wählen'], async () => {
                    try {
                        const path = await ChooseVaultIcon();
                        if (path) { m.icon = path; render(); }
                    } catch (err) {
                        toast('Icon konnte nicht geladen werden: ' + describeError(err), 'error');
                    }
                }),
                m.icon ? button('btn btn-ghost btn-sm', ['Entfernen'], () => { m.icon = ''; render(); }) : null,
            ]),
        ]),
        importing ? renderImportFilePickers(m) : null,
        m.mode === 'edit' ? renderEnvImportSection(m) : null,
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Abbrechen'], close),
            importing
                ? button('btn btn-primary', [m.busy ? 'Analysiere …' : 'Weiter'], () => analyseImport(m))
                : button('btn btn-primary', [m.mode === 'create' ? 'Anlegen' : 'Speichern'], save),
        ]),
    ], close);
}

function openConvertModal() {
    const vault = activeVault();
    if (!vault || state.selected.size === 0) return;

    const indices = [...state.selected].sort((a, b) => a - b);
    state.contextMenu = null;
    state.convertModal = {
        prefix: '',
        items: indices.map(i => ({
            index: i,
            currentKey: vault.entries[i].key || '',
            suffix: vault.entries[i].key || '',
        })),
    };
    render();
}

function renderConvertModal() {
    const m = state.convertModal;
    const close = () => { state.convertModal = null; render(); };

    const prefixInput = el('input', 'modal-input');
    prefixInput.type = 'text';
    prefixInput.placeholder = 'POSTGRES_';
    prefixInput.value = m.prefix;
    prefixInput.dataset.focusKey = 'convert-prefix';
    prefixInput.oninput = (e) => {
        m.prefix = e.target.value;
        // Wenn der Key mit dem Prefix beginnt, Suffix automatisch kuerzen.
        for (const item of m.items) {
            if (m.prefix && item.currentKey.startsWith(m.prefix)) {
                item.suffix = item.currentKey.slice(m.prefix.length);
            }
        }
        render();
    };

    const list = el('div', 'modal-list', []);
    m.items.forEach((item, i) => {
        const input = el('input', 'modal-input');
        input.type = 'text';
        input.value = item.suffix;
        input.style.fontFamily = 'var(--font-mono)';
        input.style.fontSize = '12px';
        input.oninput = (e) => { m.items[i].suffix = e.target.value; };
        list.appendChild(el('div', 'modal-list-row', [
            el('span', 'modal-list-key', [item.currentKey]),
            el('span', 'modal-arrow', [icon('arrowRight', 14)]),
            input,
        ]));
    });

    const apply = async () => {
        const prefix = m.prefix.trim();
        if (!prefix) {
            toast('Bitte ein Prefix angeben.', 'error');
            return;
        }
        const byIndex = new Map(m.items.map(it => [it.index, it.suffix.trim()]));
        const ok = await mutateEntries(entries => {
            byIndex.forEach((suffix, i) => {
                if (!entries[i]) return;
                entries[i] = { ...entries[i], key: prefix + suffix, groupPrefix: prefix };
            });
            return entries;
        }, 'Zu Gruppe zusammengefasst');

        state.convertModal = null;
        if (ok) clearSelection();
        render();
    };

    return modalShell('modal-wide', [
        el('div', 'modal-title', ['Zu Gruppe zusammenfassen']),
        el('div', 'modal-desc', [
            'Die ausgewählten Keys bekommen ein gemeinsames Prefix und erscheinen künftig als ein zusammenklappbarer Block.',
        ]),
        el('div', 'modal-field', [el('label', 'modal-label', ['Prefix']), prefixInput]),
        el('div', 'modal-field', [el('label', 'modal-label', ['Neue Key-Namen']), list]),
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Abbrechen'], close),
            button('btn btn-primary', ['Zusammenfassen'], apply),
        ]),
    ], close);
}

function renderDuplicateModal() {
    const m = state.duplicateModal;
    const close = () => { state.duplicateModal = null; render(); };
    const label = ENV_LABEL[m.target];

    return modalShell('', [
        el('div', 'modal-title', [`DEV-Werte nach ${label} übernehmen?`]),
        el('div', 'modal-warning', [
            `${m.overwrites.length} von ${m.count} ausgewählten Keys haben in ${label} bereits einen Wert. `
            + 'Diese Werte werden überschrieben.',
            el('div', 'modal-key-chips', m.overwrites.map(k => el('span', 'modal-key-chip', [k]))),
        ]),
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Abbrechen'], close),
            button('btn btn-primary', [`Nach ${label} übernehmen`], () => applyDuplicate(m.target)),
        ]),
    ], close);
}

function renderConfirmModal() {
    const m = state.confirmModal;
    const close = () => { state.confirmModal = null; render(); };

    return modalShell('', [
        el('div', 'modal-title', [m.title]),
        el('div', 'modal-desc', [m.description]),
        m.keys && m.keys.length > 0
            ? el('div', 'modal-key-chips', m.keys.slice(0, 12).map(k => el('span', 'modal-key-chip', [k])))
            : null,
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Abbrechen'], close),
            button('btn btn-danger-solid', [m.confirmLabel || 'Löschen'], async () => {
                state.confirmModal = null;
                await m.action();
                render();
            }),
        ]),
    ], close);
}

/* ==========================================================================
   Import-Assistent: aus .env-Dateien einen Vault aufbauen
   ========================================================================== */

const IMPORT_SLOTS = [
    { key: 'example', label: '.env.example', hint: 'gibt die Struktur vor', required: true },
    { key: 'dev', label: 'DEV-Werte', hint: 'optional', env: 'dev' },
    { key: 'staging', label: 'STAGING-Werte', hint: 'optional', env: 'staging' },
    { key: 'prod', label: 'PROD-Werte', hint: 'optional', env: 'prod' },
];

function renderImportFilePickers(m) {
    const wrap = el('div', 'modal-field', [
        el('label', 'modal-label', ['Dateien']),
    ]);

    for (const slot of IMPORT_SLOTS) {
        const picked = m.files[slot.key];
        const row = el('div', 'file-slot' + (picked ? ' is-filled' : ''), []);

        row.appendChild(el('span', 'file-slot-icon', [icon(picked ? 'file' : 'upload', 15)]));
        row.appendChild(el('div', 'file-slot-main', [
            el('div', 'file-slot-label', [
                slot.label,
                slot.env ? el('span', 'env-dot for-' + slot.env) : null,
            ]),
            el('div', 'file-slot-meta', [
                picked ? picked.name + ' · ' + picked.count + ' Variablen' : slot.hint,
            ]),
        ]));

        if (picked) {
            row.appendChild(iconButton('btn-icon', 'x', () => {
                m.files[slot.key] = null;
                render();
            }, 'Datei entfernen', 14));
        } else {
            row.appendChild(button('btn btn-ghost btn-sm', ['Wählen'], () => pickImportFile(m, slot)));
        }

        wrap.appendChild(row);
    }

    return wrap;
}

// countAssignments only feeds the "N Variablen" hint next to a chosen file.
// The authoritative parsing happens in Go, so the two can never disagree about
// anything that matters.
function countAssignments(text) {
    let n = 0;
    for (const raw of (text || '').split(/\r?\n/)) {
        const line = raw.trim();
        if (!line || line.startsWith('#')) continue;
        if (line.includes('=')) n++;
    }
    return n;
}

async function pickImportFile(m, slot) {
    try {
        const picked = await ChooseEnvFile(slot.label + ' auswählen');
        if (!picked) return; // abgebrochen
        m.files[slot.key] = {
            name: picked.name,
            content: picked.content,
            count: countAssignments(picked.content),
        };
        render();
    } catch (err) {
        toast('Datei konnte nicht gelesen werden: ' + describeError(err), 'error');
    }
}

async function analyseImport(m) {
    if (!(m.name || '').trim()) {
        toast('Bitte zuerst einen Namen angeben.', 'error');
        return;
    }
    if (!m.files.example) {
        toast('Für den Import wird eine .env.example benötigt.', 'error');
        return;
    }

    m.busy = true;
    render();

    try {
        const analysis = await AnalyseEnvImport(
            m.files.example.content,
            m.files.dev ? m.files.dev.content : '',
            m.files.staging ? m.files.staging.content : '',
            m.files.prod ? m.files.prod.content : '',
        );

        state.importReview = {
            name: m.name.trim(),
            description: m.description || '',
            icon: m.icon || '',
            analysis,
            // Manuell nachgetragene Werte, je Umgebung.
            fills: { dev: {}, staging: {}, prod: {} },
            // Unbekannte Keys, die in die Struktur uebernommen werden sollen.
            adopt: {},
            busy: false,
        };
        state.vaultModal = null;
    } catch (err) {
        toast('Analyse fehlgeschlagen: ' + describeError(err), 'error');
    } finally {
        m.busy = false;
        render();
    }
}

// groupPrefixFor finds the detected group a key belongs to, if any.
function groupPrefixFor(analysis, key) {
    for (const g of analysis.groups || []) {
        if ((g.keys || []).includes(key)) return g.prefix;
    }
    // Nachtraeglich uebernommene Keys erben das Prefix einer passenden Gruppe.
    for (const g of analysis.groups || []) {
        if (key.startsWith(g.prefix)) return g.prefix;
    }
    return '';
}

// adoptedKeys returns the unknown keys the user chose to keep.
function adoptedKeys(review) {
    return Object.keys(review.adopt).filter(k => review.adopt[k]);
}

// valueFor resolves one cell: the file wins, a manual entry fills the gap.
function valueFor(review, env, key) {
    const report = review.analysis.envs[env] || {};
    const fromFile = (report.values || {})[key];
    if (fromFile !== undefined && fromFile !== '') return fromFile;
    return (review.fills[env] || {})[key] || '';
}

function buildImportEntries(review) {
    const keys = [...review.analysis.keys, ...adoptedKeys(review)];
    return keys.map(key => ({
        key,
        groupPrefix: groupPrefixFor(review.analysis, key),
        valueDev: valueFor(review, 'dev', key),
        valueStage: valueFor(review, 'staging', key),
        valueProd: valueFor(review, 'prod', key),
        type: 'secret',
    }));
}

function renderImportReview() {
    const review = state.importReview;
    const analysis = review.analysis;
    const close = () => { state.importReview = null; render(); };

    const body = el('div', null, []);

    // --- Struktur-Vorschau ---
    const structure = el('div', 'import-structure', []);
    for (const g of analysis.groups || []) {
        structure.appendChild(el('div', 'import-group', [
            el('span', 'import-group-prefix', [g.prefix.replace(/_$/, '')]),
            el('span', 'group-badge', [String(g.keys.length)]),
        ]));
    }
    for (const k of analysis.ungrouped || []) {
        structure.appendChild(el('div', 'import-single', [k]));
    }

    const adopted = adoptedKeys(review);
    const totalKeys = analysis.keys.length + adopted.length;
    const groupCount = (analysis.groups || []).length;

    body.appendChild(el('div', 'modal-field', [
        el('label', 'modal-label', ['Struktur · ' + totalKeys + ' Keys']),
        el('div', 'modal-desc', [
            groupCount > 0
                ? groupCount + (groupCount === 1 ? ' Gruppe erkannt.' : ' Gruppen erkannt.')
                    + ' Keys ohne Partner mit gleichem Prefix bleiben einzeln.'
                : 'Keine gemeinsamen Prefixe gefunden – alle Keys bleiben einzeln.',
        ]),
        structure,
    ]));

    // --- Pro Umgebung ---
    let anyIssue = false;

    for (const env of ENVS) {
        const report = analysis.envs[env];
        if (!report || !report.provided) continue;

        const section = el('div', 'import-env', []);
        section.appendChild(el('div', 'import-env-head', [
            el('span', 'env-dot for-' + env),
            el('span', 'import-env-title', [ENV_LABEL[env]]),
        ]));

        const missing = report.missing || [];
        const unknown = report.unknown || [];
        const stillMissing = missing.filter(k => !(review.fills[env] || {})[k]);

        if (stillMissing.length === 0 && unknown.length === 0) {
            section.appendChild(el('div', 'import-ok', [
                icon('check', 14),
                'Stimmt mit der Struktur überein.',
            ]));
        } else {
            anyIssue = true;
        }

        // Fehlende Werte lassen sich hier direkt nachtragen, damit niemand
        // seine Dateien anfassen und von vorn beginnen muss.
        if (missing.length > 0) {
            section.appendChild(el('div', 'import-issue-label', [
                stillMissing.length + ' von ' + missing.length + ' Keys ohne Wert',
            ]));

            const list = el('div', 'import-fill-list', []);
            for (const key of missing) {
                const input = el('input', 'add-input');
                input.type = 'text';
                input.placeholder = 'Wert nachtragen (optional)';
                input.value = (review.fills[env] || {})[key] || '';
                input.dataset.focusKey = 'fill-' + env + '-' + key;
                input.oninput = (e) => {
                    review.fills[env][key] = e.target.value;
                };
                input.onblur = () => render();

                list.appendChild(el('div', 'import-fill-row', [
                    el('span', 'import-fill-key', [key]),
                    input,
                ]));
            }
            section.appendChild(list);
        }

        // Unbekannte Keys: der Nutzer entscheidet, ob sie in die Struktur sollen.
        if (unknown.length > 0) {
            section.appendChild(el('div', 'import-issue-label', [
                unknown.length + (unknown.length === 1
                    ? ' Key ist nicht in der Struktur'
                    : ' Keys sind nicht in der Struktur'),
            ]));

            const list = el('div', 'import-adopt-list', []);
            for (const key of unknown) {
                const checked = !!review.adopt[key];
                list.appendChild(el('div', 'import-adopt-row', [
                    button('row-check' + (checked ? ' is-checked' : ''), [icon('check', 11)], () => {
                        review.adopt[key] = !checked;
                        render();
                    }, 'In die Struktur übernehmen'),
                    el('span', 'import-fill-key', [key]),
                    el('span', 'import-adopt-hint', [
                        checked ? 'wird übernommen' : 'wird verworfen',
                    ]),
                ]));
            }
            section.appendChild(list);
        }

        body.appendChild(section);
    }

    const create = async () => {
        review.busy = true;
        render();
        try {
            const created = await CreateVaultFromImport(
                review.name, review.description, review.icon, buildImportEntries(review));
            state.vaults.push(created);
            state.vaults.sort((a, b) => (a.name || '').localeCompare(b.name || ''));
            state.activeVaultId = created.id;
            state.importReview = null;
            toast('Vault mit ' + created.entries.length + ' Keys angelegt', 'success');
        } catch (err) {
            toast('Import fehlgeschlagen: ' + describeError(err), 'error');
        } finally {
            review.busy = false;
            render();
        }
    };

    return modalShell('modal-wide', [
        el('div', 'modal-title', ['„' + review.name + '“ importieren']),
        el('div', 'modal-desc', [
            anyIssue
                ? 'Es gibt Abweichungen zwischen den Wert-Dateien und der Struktur. Du kannst sie hier direkt beheben – die Reihenfolge in den Dateien spielt keine Rolle.'
                : 'Alle Dateien passen zur Struktur.',
        ]),
        body,
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Zurück'], () => {
                state.importReview = null;
                openCreateVault();
            }),
            button('btn btn-primary', [review.busy ? 'Lege an …' : 'Vault anlegen'], create),
        ]),
    ], close);
}

/* ==========================================================================
   Werte nachträglich in einen bestehenden Vault importieren
   ========================================================================== */

// renderEnvImportSection shows, per environment, how much of the vault is
// already filled and offers to import the rest from a file. The fill counts
// are the point: they say at a glance which environment still needs values.
function renderEnvImportSection(m) {
    const target = state.vaults.find(v => v.id === m.id);
    if (!target) return null;

    const entries = target.entries || [];
    const total = entries.length;
    if (total === 0) return null;

    const rows = el('div', 'env-import-list', []);

    for (const env of ENVS) {
        const filled = entries.filter(e => (e[ENV_FIELD[env]] || '') !== '').length;
        const empty = filled === 0;

        rows.appendChild(el('div', 'env-import-row' + (empty ? ' is-empty' : ''), [
            el('span', 'env-dot for-' + env),
            el('div', 'env-import-main', [
                el('div', 'env-import-label', [ENV_LABEL[env]]),
                el('div', 'env-import-meta', [
                    empty ? 'noch keine Werte' : filled + ' von ' + total + ' belegt',
                ]),
            ]),
            button('btn btn-ghost btn-sm', [icon('upload', 13), 'Datei'],
                () => pickEnvImport(target, env),
                ENV_LABEL[env] + '-Werte aus einer Datei importieren'),
        ]));
    }

    return el('div', 'modal-field', [
        el('label', 'modal-label', ['Werte importieren']),
        el('div', 'modal-desc', [
            'Eine .env-Datei hochladen und einer Umgebung zuordnen. Die Keys dieses Vaults geben dabei die Struktur vor.',
        ]),
        rows,
    ]);
}

async function pickEnvImport(target, env) {
    try {
        const picked = await ChooseEnvFile(ENV_LABEL[env] + '-Werte auswählen');
        if (!picked) return;

        const analysis = await AnalyseEnvForVault(target.id, env, picked.content);

        state.envImport = {
            vaultId: target.id,
            vaultName: target.name,
            env,
            fileName: picked.name,
            analysis,
            fills: {},
            adopt: {},
            overwrite: false,
            busy: false,
        };
        state.vaultModal = null;
        render();
    } catch (err) {
        toast('Import fehlgeschlagen: ' + describeError(err), 'error');
    }
}

// buildEnvImportValues decides what actually gets written.
function buildEnvImportValues(imp) {
    const analysis = imp.analysis;
    const unknown = new Set(analysis.unknown || []);
    const willOverwrite = new Set(analysis.overwrite || []);
    const values = {};

    for (const [key, value] of Object.entries(analysis.values || {})) {
        // Ein unbekannter Key kommt nur mit, wenn er uebernommen werden soll.
        if (unknown.has(key) && !imp.adopt[key]) continue;
        // Vorhandene Werte bleiben stehen, solange nicht ausdruecklich
        // ueberschrieben werden soll.
        if (willOverwrite.has(key) && !imp.overwrite) continue;
        values[key] = value;
    }

    // Manuell nachgetragene Werte fuellen die Luecken.
    for (const [key, value] of Object.entries(imp.fills || {})) {
        if (value) values[key] = value;
    }

    return values;
}

function renderEnvImportReview() {
    const imp = state.envImport;
    const analysis = imp.analysis;
    const close = () => { state.envImport = null; render(); };

    const missing = analysis.missing || [];
    const unknown = analysis.unknown || [];
    const conflicts = analysis.overwrite || [];

    const body = el('div', null, []);

    const planned = Object.keys(buildEnvImportValues(imp)).length;
    body.appendChild(el('div', 'import-summary', [
        el('span', 'env-dot for-' + imp.env),
        planned + (planned === 1 ? ' Wert wird gesetzt' : ' Werte werden gesetzt'),
        el('span', 'import-summary-file', [imp.fileName]),
    ]));

    // Konflikte zuerst: hier steht etwas auf dem Spiel.
    if (conflicts.length > 0) {
        const warn = el('div', 'modal-warning', [
            conflicts.length + (conflicts.length === 1
                ? ' Key hat in ' + ENV_LABEL[imp.env] + ' bereits einen Wert.'
                : ' Keys haben in ' + ENV_LABEL[imp.env] + ' bereits einen Wert.'),
            el('div', 'modal-key-chips', conflicts.slice(0, 12).map(k => el('span', 'modal-key-chip', [k]))),
        ]);
        body.appendChild(warn);

        body.appendChild(el('div', 'import-adopt-row', [
            button('row-check' + (imp.overwrite ? ' is-checked' : ''), [icon('check', 11)], () => {
                imp.overwrite = !imp.overwrite;
                render();
            }, 'Vorhandene Werte überschreiben'),
            el('span', null, [
                imp.overwrite
                    ? 'Vorhandene Werte werden überschrieben'
                    : 'Vorhandene Werte bleiben unangetastet',
            ]),
        ]));
    }

    if (missing.length > 0) {
        const open = missing.filter(k => !(imp.fills[k] || ''));
        body.appendChild(el('div', 'import-issue-label', [
            open.length + ' von ' + missing.length + ' Keys ohne Wert in dieser Datei',
        ]));

        const list = el('div', 'import-fill-list', []);
        for (const key of missing) {
            const input = el('input', 'add-input');
            input.type = 'text';
            input.placeholder = 'Wert nachtragen (optional)';
            input.value = imp.fills[key] || '';
            input.dataset.focusKey = 'envfill-' + key;
            input.oninput = (e) => { imp.fills[key] = e.target.value; };
            input.onblur = () => render();

            list.appendChild(el('div', 'import-fill-row', [
                el('span', 'import-fill-key', [key]),
                input,
            ]));
        }
        body.appendChild(list);
    }

    if (unknown.length > 0) {
        body.appendChild(el('div', 'import-issue-label', [
            unknown.length + (unknown.length === 1
                ? ' Key aus der Datei fehlt im Vault'
                : ' Keys aus der Datei fehlen im Vault'),
        ]));

        const list = el('div', 'import-adopt-list', []);
        for (const key of unknown) {
            const checked = !!imp.adopt[key];
            list.appendChild(el('div', 'import-adopt-row', [
                button('row-check' + (checked ? ' is-checked' : ''), [icon('check', 11)], () => {
                    imp.adopt[key] = !checked;
                    render();
                }, 'Key im Vault anlegen'),
                el('span', 'import-fill-key', [key]),
                el('span', 'import-adopt-hint', [
                    checked ? 'wird angelegt' : 'wird verworfen',
                ]),
            ]));
        }
        body.appendChild(list);
    }

    if (missing.length === 0 && unknown.length === 0 && conflicts.length === 0) {
        body.appendChild(el('div', 'import-ok', [
            icon('check', 14),
            'Die Datei passt genau zu den Keys dieses Vaults.',
        ]));
    }

    const apply = async () => {
        const values = buildEnvImportValues(imp);
        if (Object.keys(values).length === 0) {
            toast('Es bleibt nichts zu übernehmen.', 'error');
            return;
        }

        imp.busy = true;
        render();
        try {
            const updated = await ApplyEnvValues(
                imp.vaultId, imp.env, values, adoptedKeys(imp));
            state.vaults = state.vaults.map(v => (v.id === updated.id ? updated : v));
            state.activeVaultId = updated.id;
            state.envImport = null;
            toast(Object.keys(values).length + ' Werte in ' + ENV_LABEL[imp.env] + ' übernommen', 'success');
        } catch (err) {
            toast('Übernehmen fehlgeschlagen: ' + describeError(err), 'error');
        } finally {
            imp.busy = false;
            render();
        }
    };

    return modalShell('modal-wide', [
        el('div', 'modal-title', [ENV_LABEL[imp.env] + '-Werte importieren']),
        el('div', 'modal-desc', [
            'Ziel: „' + imp.vaultName + '“. Die Keys des Vaults geben die Struktur vor; verglichen wird nach Namen, nicht nach Reihenfolge.',
        ]),
        body,
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Abbrechen'], close),
            button('btn btn-primary', [imp.busy ? 'Übernehme …' : 'Übernehmen'], apply),
        ]),
    ], close);
}

/* ==========================================================================
   Backup und Wiederherstellung
   ========================================================================== */

function openBackupModal() {
    state.backupModal = {
        view: 'menu',
        passphrase: '',
        repeat: '',
        replace: false,
        minLength: 12,
        busy: false,
    };
    render();

    MinBackupPassphraseLength()
        .then(n => {
            if (state.backupModal) {
                state.backupModal.minLength = n;
                render();
            }
        })
        .catch(() => { /* Der Standardwert bleibt stehen. */ });
}

function backupChoice(iconName, title, text, onClick) {
    const row = el('div', 'backup-choice', [
        el('span', 'backup-choice-icon', [icon(iconName, 18)]),
        el('div', null, [
            el('div', 'backup-choice-title', [title]),
            el('div', 'backup-choice-text', [text]),
        ]),
    ]);
    row.onclick = onClick;
    return row;
}

function renderBackupModal() {
    const m = state.backupModal;
    const close = () => { state.backupModal = null; render(); };

    if (m.view === 'menu') {
        return modalShell('', [
            el('div', 'modal-title', ['Backup']),
            el('div', 'modal-desc', [
                'Die Vault-Datei ist an dieses Benutzerkonto gebunden. Ein Backup wird stattdessen mit einer Passphrase geschützt und lässt sich dadurch auch auf einem anderen Rechner wiederherstellen.',
            ]),
            el('div', 'backup-choice-list', [
                backupChoice('download', 'Backup erstellen',
                    'Alle Vaults verschlüsselt in eine Datei schreiben.',
                    () => { m.view = 'create'; render(); }),
                backupChoice('upload', 'Backup wiederherstellen',
                    'Vaults aus einer Backup-Datei zurückholen.',
                    () => { m.view = 'restore'; render(); }),
            ]),
            el('div', 'modal-actions', [button('btn btn-ghost', ['Schließen'], close)]),
        ], close);
    }

    if (m.view === 'create') {
        const pw = el('input', 'modal-input');
        pw.type = 'password';
        pw.value = m.passphrase;
        pw.placeholder = 'mindestens ' + m.minLength + ' Zeichen';
        pw.dataset.focusKey = 'backup-pw';
        pw.oninput = (e) => { m.passphrase = e.target.value; };

        const repeat = el('input', 'modal-input');
        repeat.type = 'password';
        repeat.value = m.repeat;
        repeat.placeholder = 'zur Kontrolle wiederholen';
        repeat.oninput = (e) => { m.repeat = e.target.value; };

        const run = async () => {
            if (m.passphrase.length < m.minLength) {
                toast('Die Passphrase braucht mindestens ' + m.minLength + ' Zeichen.', 'error');
                return;
            }
            if (m.passphrase !== m.repeat) {
                toast('Die beiden Passphrasen stimmen nicht überein.', 'error');
                return;
            }
            try {
                const path = await ChooseBackupTarget();
                if (!path) return;
                m.busy = true;
                render();
                const summary = await BackupVaults(path, m.passphrase);
                state.backupModal = null;
                toast(summary, 'success');
            } catch (err) {
                toast('Backup fehlgeschlagen: ' + describeError(err), 'error');
            } finally {
                m.busy = false;
                render();
            }
        };

        return modalShell('', [
            el('div', 'modal-title', ['Backup erstellen']),
            el('div', 'modal-warning', [
                'Ohne die Passphrase lässt sich das Backup von niemandem öffnen – auch nicht von dir. Bewahre sie getrennt von der Datei auf.',
            ]),
            el('div', 'modal-field', [el('label', 'modal-label', ['Passphrase']), pw]),
            el('div', 'modal-field', [el('label', 'modal-label', ['Wiederholen']), repeat]),
            el('div', 'modal-actions', [
                button('btn btn-ghost', ['Zurück'], () => { m.view = 'menu'; render(); }),
                button('btn btn-primary', [m.busy ? 'Sichere …' : 'Datei wählen und sichern'], run),
            ]),
        ], close);
    }

    // Wiederherstellen
    const pw = el('input', 'modal-input');
    pw.type = 'password';
    pw.value = m.passphrase;
    pw.placeholder = 'Passphrase des Backups';
    pw.dataset.focusKey = 'restore-pw';
    pw.oninput = (e) => { m.passphrase = e.target.value; };

    const run = async () => {
        if (!m.passphrase) {
            toast('Bitte die Passphrase eingeben.', 'error');
            return;
        }
        try {
            const path = await ChooseBackupSource();
            if (!path) return;
            m.busy = true;
            render();
            const result = await RestoreVaults(path, m.passphrase, m.replace);
            state.backupModal = null;
            await loadVaults();
            const added = (result.added || []).length;
            toast(added + (added === 1 ? ' Vault' : ' Vaults') + ' wiederhergestellt', 'success');
            for (const skipped of result.skipped || []) {
                toast('Übersprungen: ' + skipped, 'error');
            }
        } catch (err) {
            toast('Wiederherstellen fehlgeschlagen: ' + describeError(err), 'error');
        } finally {
            m.busy = false;
            render();
        }
    };

    return modalShell('', [
        el('div', 'modal-title', ['Backup wiederherstellen']),
        el('div', 'modal-desc', [
            m.replace
                ? 'Alle vorhandenen Vaults werden durch den Inhalt des Backups ersetzt.'
                : 'Vaults aus dem Backup werden ergänzt. Solche, deren ID oder Name schon existiert, werden übersprungen.',
        ]),
        el('div', 'modal-field', [el('label', 'modal-label', ['Passphrase']), pw]),
        el('div', 'modal-field', [
            el('div', 'import-adopt-row', [
                button('row-check' + (m.replace ? ' is-checked' : ''), [icon('check', 11)], () => {
                    m.replace = !m.replace;
                    render();
                }, 'Vorhandene Vaults ersetzen'),
                el('span', null, ['Vorhandene Vaults ersetzen statt ergänzen']),
            ]),
        ]),
        m.replace
            ? el('div', 'modal-warning', ['Das kann nicht rückgängig gemacht werden.'])
            : null,
        el('div', 'modal-actions', [
            button('btn btn-ghost', ['Zurück'], () => { m.view = 'menu'; render(); }),
            button(m.replace ? 'btn btn-danger-solid' : 'btn btn-primary',
                [m.busy ? 'Stelle wieder her …' : 'Datei wählen und wiederherstellen'], run),
        ]),
    ], close);
}

/* ==========================================================================
   Kontextmenü und Toasts
   ========================================================================== */

function renderContextMenu() {
    const m = state.contextMenu;
    const menu = el('div', 'context-menu', []);

    for (const item of m.items) {
        if (item.separator) {
            menu.appendChild(el('div', 'context-sep'));
            continue;
        }
        const node = el('div', 'context-item' + (item.danger ? ' is-danger' : ''), [
            icon(item.iconName, 14),
            item.label,
        ]);
        node.onclick = () => {
            state.contextMenu = null;
            item.action();
        };
        menu.appendChild(node);
    }

    // Erst nach dem Einhaengen positionieren, damit das Menue nicht aus dem
    // Fenster laeuft.
    menu.style.left = '0px';
    menu.style.top = '0px';
    menu.style.visibility = 'hidden';
    requestAnimationFrame(() => {
        const rect = menu.getBoundingClientRect();
        const x = Math.min(m.x, window.innerWidth - rect.width - 8);
        const y = Math.min(m.y, window.innerHeight - rect.height - 8);
        menu.style.left = `${Math.max(8, x)}px`;
        menu.style.top = `${Math.max(8, y)}px`;
        menu.style.visibility = 'visible';
    });

    return menu;
}

function renderToasts() {
    const stack = el('div', 'toast-stack', []);
    for (const t of state.toasts) {
        stack.appendChild(el('div', `toast is-${t.kind}`, [
            t.kind !== 'info' ? el('span', 'toast-icon', [icon(t.kind === 'error' ? 'alert' : 'check', 15)]) : null,
            t.message,
        ]));
    }
    return stack;
}

function renderStartupBanner() {
    return el('div', 'startup-banner', [
        el('span', 'startup-banner-icon', [icon('alert', 18)]),
        el('div', null, [
            el('div', 'startup-banner-title', ['Vault-Daten konnten nicht gelesen werden']),
            el('div', 'startup-banner-text', [
                'Änderungen werden nicht gespeichert, damit die vorhandene verschlüsselte Datei nicht überschrieben wird. '
                + 'Häufigste Ursache: die Vaults wurden mit einem anderen Protector verschlüsselt, etwa ein altes master.key-Setup, das jetzt unter DPAPI läuft.',
            ]),
            el('div', 'startup-banner-detail', [state.startupError]),
        ]),
    ]);
}

/* ==========================================================================
   Render
   ========================================================================== */

// Fokus und Scrollposition ueberleben den Neuaufbau des DOM.
function captureUiState() {
    const active = document.activeElement;
    const focusKey = active && active.dataset ? active.dataset.focusKey : null;
    const selStart = active && typeof active.selectionStart === 'number' ? active.selectionStart : null;
    const scroller = document.querySelector('[data-scroll-key="table"]');
    return { focusKey, selStart, scrollTop: scroller ? scroller.scrollTop : 0 };
}

function restoreUiState(snapshot) {
    const scroller = document.querySelector('[data-scroll-key="table"]');
    if (scroller && snapshot.scrollTop) scroller.scrollTop = snapshot.scrollTop;

    let target = null;
    if (state.editing) {
        const key = state.editing.field === 'key' || state.editing.field === 'type'
            ? `${state.editing.field}-${state.editing.index}`
            : `${state.editing.field}-${state.editing.index}`;
        target = document.querySelector(`[data-focus-key="${key}"]`);
    }
    if (!target && state.addPanel && typeof state.addPanel.focusIndex === 'number') {
        target = document.querySelector(`[data-focus-key="add-key-${state.addPanel.focusIndex}"]`);
        state.addPanel.focusIndex = null;
    }
    if (!target && snapshot.focusKey) {
        target = document.querySelector(`[data-focus-key="${snapshot.focusKey}"]`);
    }
    if (!target && state.vaultModal) {
        target = document.querySelector('[data-focus-key="vault-name"]');
    }

    if (target) {
        target.focus();
        if (typeof target.setSelectionRange === 'function') {
            if (snapshot.focusKey === target.dataset.focusKey && snapshot.selStart !== null) {
                target.setSelectionRange(snapshot.selStart, snapshot.selStart);
            } else {
                target.setSelectionRange(target.value.length, target.value.length);
            }
        }
    }
}

function render() {
    const snapshot = captureUiState();

    app.replaceChildren();

    const shell = el('div', 'app-shell', []);
    if (state.startupError) shell.appendChild(renderStartupBanner());
    shell.appendChild(renderTopbar());
    shell.appendChild(el('div', 'app-body', [renderSidebar(), renderMain()]));
    app.appendChild(shell);

    if (state.vaultModal) app.appendChild(renderVaultModal());
    if (state.importReview) app.appendChild(renderImportReview());
    if (state.envImport) app.appendChild(renderEnvImportReview());
    if (state.backupModal) app.appendChild(renderBackupModal());
    if (state.convertModal) app.appendChild(renderConvertModal());
    if (state.duplicateModal) app.appendChild(renderDuplicateModal());
    if (state.confirmModal) app.appendChild(renderConfirmModal());
    if (state.selected.size > 0) app.appendChild(renderSelectionBar());
    if (state.contextMenu) app.appendChild(renderContextMenu());
    if (state.toasts.length > 0) app.appendChild(renderToasts());

    restoreUiState(snapshot);
}

// Die Suche tippt sich fluessiger, wenn nicht bei jedem Zeichen der komplette
// Baum inklusive Sidebar neu gebaut wird.
function renderMainOnly() {
    const snapshot = captureUiState();
    const body = document.querySelector('.app-body');
    const old = body ? body.querySelector('.main') : null;
    if (!body || !old) { render(); return; }
    old.replaceWith(renderMain());

    const existingBar = app.querySelector('.selection-bar');
    if (existingBar) existingBar.remove();
    if (state.selected.size > 0) app.appendChild(renderSelectionBar());

    restoreUiState(snapshot);
}

/* ==========================================================================
   Globale Tastatur und Klicks
   ========================================================================== */

document.addEventListener('click', () => {
    if (state.contextMenu) {
        state.contextMenu = null;
        render();
    }
});

// Eigenes Kontextmenue nur dort, wo wir wirklich eines anbieten.
document.addEventListener('contextmenu', (e) => {
    if (!e.target.closest('.key-row-entry, .vault-item')) {
        e.preventDefault();
    }
});

document.addEventListener('keydown', (e) => {
    const typing = ['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName);

    if (e.key === 'Escape') {
        if (state.contextMenu) { state.contextMenu = null; render(); return; }
        if (state.confirmModal) { state.confirmModal = null; render(); return; }
        if (state.duplicateModal) { state.duplicateModal = null; render(); return; }
        if (state.convertModal) { state.convertModal = null; render(); return; }
        if (state.backupModal) { state.backupModal = null; render(); return; }
        if (state.envImport) { state.envImport = null; render(); return; }
        if (state.importReview) { state.importReview = null; render(); return; }
        if (state.vaultModal) { state.vaultModal = null; render(); return; }
        if (state.editing) { state.editing = null; render(); return; }
        if (state.addPanel) { state.addPanel = null; render(); return; }
        if (state.selected.size > 0) { clearSelection(); render(); return; }
        if (state.search) { state.search = ''; render(); }
        return;
    }

    if (typing) return;

    if (e.key === '/') {
        e.preventDefault();
        const input = document.querySelector('[data-focus-key="search"]');
        if (input) input.focus();
        return;
    }

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
        e.preventDefault();
        const input = document.querySelector('[data-focus-key="search"]');
        if (input) { input.focus(); input.select(); }
        return;
    }

    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'n') {
        e.preventDefault();
        if (activeVault()) openAddPanel('single');
    }
});

loadVaults();
