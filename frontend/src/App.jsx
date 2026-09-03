import {Component, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Terminal} from '@xterm/xterm';
import {FitAddon} from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import './App.css';
import {
    CancelSFTPTransfer, ChooseFilesSFTP, CopySFTP, CreateDirectorySFTP, DeleteHost, DeleteSFTP,
    DownloadSFTP, GetHosts, ListSFTP, RenameSFTP, ResizeTerminal, ResolveHostSave, StartSSHSession,
    StartTerminal, StopTerminal, ToggleFavorite, UploadClipboardFileSFTP,
    UploadPathsSFTP, UpsertHost, WriteTerminal,
} from '../wailsjs/go/main/App';
import {EventsOff, EventsOn, OnFileDrop, OnFileDropOff, Quit, WindowMinimise, WindowToggleMaximise} from '../wailsjs/runtime/runtime';

const iconPaths = {
    terminal: <><path d="m4 17 6-5-6-5"/><path d="M12 19h8"/></>,
    server: <><rect x="4" y="4" width="16" height="6" rx="2"/><rect x="4" y="14" width="16" height="6" rx="2"/><path d="M8 7h.01M8 17h.01"/></>,
    folder: <path d="M3 6.5A2.5 2.5 0 0 1 5.5 4H9l2 2h7.5A2.5 2.5 0 0 1 21 8.5v8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 16.5z"/>,
    folderOpen: <><path d="M3 7V6a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v1"/><path d="M3 9h18l-2 10H5z"/></>,
    file: <><path d="M6 3h8l4 4v14H6z"/><path d="M14 3v5h5"/></>,
    search: <><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></>,
    star: <path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9z"/>,
    trash: <><path d="M4 7h16M9 7V4h6v3M7 7l1 14h8l1-14"/><path d="M10 11v6M14 11v6"/></>,
    upload: <><path d="M12 16V4m-4 4 4-4 4 4"/><path d="M5 14v6h14v-6"/></>,
    download: <><path d="M12 4v12m-4-4 4 4 4-4"/><path d="M5 20h14"/></>,
    refresh: <><path d="M20 7v5h-5"/><path d="M4 17v-5h5"/><path d="M6.1 8a7 7 0 0 1 11.6-2L20 8M4 16l2.3 2a7 7 0 0 0 11.6-2"/></>,
    chevron: <path d="m9 18 6-6-6-6"/>, plus: <path d="M12 5v14M5 12h14"/>,
    copy: <><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></>,
    lock: <><rect x="5" y="10" width="14" height="10" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/></>,
    settings: <><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1z"/></>,
    edit: <><path d="m4 20 4.2-1 10.6-10.6a2 2 0 0 0-2.8-2.8L5.4 16.2z"/><path d="m14.5 7.1 2.8 2.8M4 20h6"/></>,
    x: <path d="m7 7 10 10M17 7 7 17"/>,
};

function Icon({name, size = 18, filled = false}) {
    return <svg width={size} height={size} viewBox="0 0 24 24" fill={filled ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{iconPaths[name]}</svg>;
}

class AppErrorBoundary extends Component {
    state = {error: null};
    static getDerivedStateFromError(error) { return {error}; }
    componentDidCatch(error) { console.error('Relay frontend error:', error); }
    render() {
        if (!this.state.error) return this.props.children;
        return <div className="fatal-error"><Icon name="terminal" size={28}/><h1>Ошибка интерфейса</h1><p>{String(this.state.error)}</p><button onClick={() => window.location.reload()}>Перезагрузить</button></div>;
    }
}

const terminalThemes = {
    dark: {background: '#0c0e12', foreground: '#d9dde7', cursor: '#67e8a7', cursorAccent: '#0c0e12', selectionBackground: '#315c4f99', black: '#171a20', red: '#ff7373', green: '#67e8a7', yellow: '#e5c07b', blue: '#78a9ff', magenta: '#c792ea', cyan: '#79dbea', white: '#d9dde7', brightBlack: '#646b78', brightRed: '#ff8d8d', brightGreen: '#86efb9', brightYellow: '#f1d08b', brightBlue: '#97bdff', brightMagenta: '#d6a5f2', brightCyan: '#9ae9f3', brightWhite: '#fff'},
    light: {background: '#ffffff', foreground: '#252a31', cursor: '#16865c', cursorAccent: '#ffffff', selectionBackground: '#9ddfc4aa', black: '#20242a', red: '#c62828', green: '#16865c', yellow: '#8a6500', blue: '#2368b5', magenta: '#7b3fa1', cyan: '#087b87', white: '#e4e7eb', brightBlack: '#6d747e', brightRed: '#e53935', brightGreen: '#218f63', brightYellow: '#a97800', brightBlue: '#3579c8', brightMagenta: '#9452b5', brightCyan: '#168e99', brightWhite: '#ffffff'},
    vs2008: {background: '#ffffff', foreground: '#000000', cursor: '#000000', cursorAccent: '#ffffff', selectionBackground: '#3399ff88', black: '#000000', red: '#a31515', green: '#008000', yellow: '#795e26', blue: '#0000ff', magenta: '#800080', cyan: '#008080', white: '#c0c0c0', brightBlack: '#666666', brightRed: '#d02020', brightGreen: '#169b16', brightYellow: '#9b791c', brightBlue: '#2b579a', brightMagenta: '#9b329b', brightCyan: '#159595', brightWhite: '#ffffff'},
    vs2012: {background: '#1e1e1e', foreground: '#dcdcdc', cursor: '#f1f1f1', cursorAccent: '#1e1e1e', selectionBackground: '#264f78', black: '#1e1e1e', red: '#f44747', green: '#6a9955', yellow: '#dcdcaa', blue: '#569cd6', magenta: '#c586c0', cyan: '#4ec9b0', white: '#d4d4d4', brightBlack: '#808080', brightRed: '#f66', brightGreen: '#b5cea8', brightYellow: '#e8e8a8', brightBlue: '#9cdcfe', brightMagenta: '#d8a0df', brightCyan: '#7ed8c4', brightWhite: '#ffffff'},
};

const defaultSettings = {theme: 'dark', terminalFontSize: 13.5, reducedMotion: false, confirmDelete: true};
const themeChoices = [
    {id: 'dark', name: 'Тёмная', hint: 'Текущее оформление'},
    {id: 'light', name: 'Светлая', hint: 'Нейтральная и контрастная'},
    {id: 'vs2008', name: 'Visual Studio 2008', hint: 'Классическая синяя'},
    {id: 'vs2012', name: 'Visual Studio 2012', hint: 'Плоская тёмная'},
];

function loadSettings() {
    try { return {...defaultSettings, ...JSON.parse(localStorage.getItem('relay:settings') || '{}')}; }
    catch (_) { return defaultSettings; }
}

const terminalOptions = {
    cursorBlink: true, cursorStyle: 'block', fontFamily: 'JetBrains Mono, SFMono-Regular, Consolas, Liberation Mono, monospace',
    fontSize: 13.5, lineHeight: 1.35, letterSpacing: 0.1, scrollback: 10000, allowTransparency: true,
};

function sessionID() {
    return (globalThis.crypto?.randomUUID?.() || `terminal-${Date.now()}-${Math.random()}`).replaceAll('.', '-');
}

function TerminalView({tab, active, register, onCommand, onReady, appearance}) {
    const element = useRef(null);
    const command = useRef('');
    const fitRef = useRef(null);

    useEffect(() => {
        const fitAddon = new FitAddon();
        const xterm = new Terminal({...terminalOptions, fontSize: appearance.terminalFontSize, theme: terminalThemes[appearance.theme] || terminalThemes.dark});
        xterm.loadAddon(fitAddon);
        xterm.open(element.current);
        const fit = () => {
            if (!element.current?.offsetParent) return;
            try { fitAddon.fit(); ResizeTerminal(tab.id, xterm.cols, xterm.rows).catch(() => {}); } catch (_) { /* layout transition */ }
        };
        fitRef.current = fit;
        register(tab.id, {terminal: xterm, fit, write: (data) => xterm.write(data), exited: () => xterm.writeln('\r\n\x1b[2m[процесс завершён]\x1b[0m')});
        const input = xterm.onData((data) => {
            if (data === '\r') { const line = command.current; command.current = ''; onCommand(tab.id, line); }
            else if (data === '\x7f') command.current = command.current.slice(0, -1);
            else if (data === '\x03' || data === '\x15') command.current = '';
            else if (!data.startsWith('\x1b') && !/[\x00-\x08\x0b-\x1f]/.test(data)) command.current += data;
            WriteTerminal(tab.id, data).catch(() => {});
        });
        const observer = new ResizeObserver(() => requestAnimationFrame(fit));
        observer.observe(element.current);
        requestAnimationFrame(async () => {
            fit();
            try {
                if (tab.kind === 'ssh') await StartSSHSession(tab.id, tab.hostId, xterm.cols, xterm.rows);
                else await StartTerminal(tab.id, xterm.cols, xterm.rows);
                onReady(tab.id, true); xterm.focus();
            } catch (error) {
                xterm.writeln(`\r\n\x1b[31mНе удалось запустить терминал: ${error}\x1b[0m`);
            }
        });
        return () => { observer.disconnect(); input.dispose(); register(tab.id, null); StopTerminal(tab.id).catch(() => {}); xterm.dispose(); };
    }, [onCommand, onReady, register, tab.hostId, tab.id, tab.kind]);

    useEffect(() => {
        if (active) requestAnimationFrame(() => { fitRef.current?.(); register(tab.id)?.terminal?.focus?.(); });
    }, [active, register, tab.id]);

    useEffect(() => {
        const api = register(tab.id);
        if (!api?.terminal) return;
        api.terminal.options.fontSize = appearance.terminalFontSize;
        api.terminal.options.theme = terminalThemes[appearance.theme] || terminalThemes.dark;
        requestAnimationFrame(() => api.fit?.());
    }, [appearance.terminalFontSize, appearance.theme, register, tab.id]);

    return <div className={`terminal-session ${active ? 'active' : ''}`}><div className="terminal-wrap" ref={element}/></div>;
}

function relativeTime(value) {
    const seconds = Math.max(0, (Date.now() - new Date(value).getTime()) / 1000);
    if (seconds < 60) return 'только что';
    if (seconds < 3600) return `${Math.floor(seconds / 60)} мин назад`;
    if (seconds < 86400) return `${Math.floor(seconds / 3600)} ч назад`;
    return new Intl.DateTimeFormat('ru', {day: 'numeric', month: 'short'}).format(new Date(value));
}

function formatSize(bytes) {
    if (!Number.isFinite(bytes) || bytes < 0) return '—';
    if (bytes === 0) return '0 Б';
    const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
    const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
}

function formatDuration(seconds) {
    if (!Number.isFinite(seconds) || seconds < 0) return '—';
    if (seconds < 60) return `${Math.max(1, Math.round(seconds))} с`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)} мин ${Math.round(seconds % 60)} с`;
    return `${Math.floor(seconds / 3600)} ч ${Math.round((seconds % 3600) / 60)} мин`;
}

function fileCount(value) {
    const tail = value % 100;
    const last = value % 10;
    if (tail >= 11 && tail <= 14) return `${value} файлов`;
    if (last === 1) return `${value} файл`;
    if (last >= 2 && last <= 4) return `${value} файла`;
    return `${value} файлов`;
}

function TransferCard({transfer, cancelling, onCancel}) {
    const complete = transfer.done && !transfer.error;
    const cancelled = transfer.done && String(transfer.error || '').toLowerCase().includes('отмен');
    const failed = transfer.done && Boolean(transfer.error) && !cancelled;
    const determinate = transfer.bytesTotal > 0;
    const percent = complete ? 100 : determinate ? Math.min(99, Math.round(transfer.bytesDone / transfer.bytesTotal * 100)) : 0;
    const seconds = Math.max(0, (transfer.elapsedMs || 0) / 1000);
    const speed = seconds > .25 ? transfer.bytesDone / seconds : 0;
    const remaining = speed > 0 && transfer.bytesTotal > transfer.bytesDone ? (transfer.bytesTotal - transfer.bytesDone) / speed : null;
    const currentFile = transfer.filesTotal ? Math.min(transfer.filesDone + (transfer.done ? 0 : 1), transfer.filesTotal) : 0;
    const action = transfer.direction === 'download' ? 'Скачивание' : 'Загрузка';
    const title = cancelled ? 'Передача отменена' : failed ? 'Ошибка передачи' : complete ? `${action} завершена` : action;

    return <div className={`transfer-card ${complete ? 'complete' : ''} ${failed ? 'failed' : ''} ${cancelled ? 'cancelled' : ''}`}>
        <div className="transfer-head">
            <span className="transfer-symbol"><Icon name={transfer.direction === 'download' ? 'download' : 'upload'} size={14}/></span>
            <span className="transfer-title"><strong>{title}</strong><small title={transfer.name}>{transfer.error || transfer.name || 'Подготовка…'}</small></span>
            <b>{determinate || transfer.done ? `${percent}%` : '…'}</b>
            {!transfer.done && <button className="transfer-cancel" onClick={onCancel} disabled={cancelling} title="Отменить передачу"><Icon name="x" size={11}/><span>{cancelling ? 'Отмена…' : 'Отменить'}</span></button>}
        </div>
        <div className={`progress-track ${determinate || transfer.done ? '' : 'indeterminate'}`} role="progressbar" aria-label={title} aria-valuemin="0" aria-valuemax="100" aria-valuenow={determinate || transfer.done ? percent : undefined}>
            <span style={determinate || transfer.done ? {width: `${percent}%`} : undefined}/>
        </div>
        <div className="transfer-meta">
            <span>{transfer.filesTotal ? `Файл ${currentFile} из ${transfer.filesTotal}` : 'Подготовка списка…'}</span>
            <span>{formatSize(transfer.bytesDone)} / {formatSize(transfer.bytesTotal)}</span>
        </div>
        {!transfer.done && speed > 0 && <div className="transfer-rate"><span>{formatSize(speed)}/с</span><span>{remaining === null ? '' : `осталось ≈ ${formatDuration(remaining)}`}</span></div>}
    </div>;
}

function Modal({title, subtitle, onClose, children, className = ''}) {
    return <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose?.(); }}>
        <section className={`modal ${className}`} role="dialog" aria-modal="true" aria-label={title}>
            <header className="modal-head"><div><h2>{title}</h2>{subtitle && <p>{subtitle}</p>}</div>{onClose && <button onClick={onClose} aria-label="Закрыть"><Icon name="x" size={15}/></button>}</header>
            {children}
        </section>
    </div>;
}

function SettingsDialog({settings, onChange, onClose}) {
    const update = (patch) => onChange({...settings, ...patch});
    return <Modal title="Настройки" subtitle="Внешний вид и поведение Relay" onClose={onClose} className="settings-modal">
        <div className="settings-body">
            <div className="settings-section"><h3>Тема</h3><div className="theme-grid">{themeChoices.map((theme) => <button key={theme.id} className={`theme-choice ${settings.theme === theme.id ? 'selected' : ''}`} onClick={() => update({theme: theme.id})}><span className={`theme-preview theme-${theme.id}`}><i/><i/><i/></span><strong>{theme.name}</strong><small>{theme.hint}</small></button>)}</div></div>
            <div className="settings-section"><h3>Терминал</h3><label className="setting-row"><span><strong>Размер шрифта</strong><small>От 11 до 18 px</small></span><span className="range-control"><input type="range" min="11" max="18" step="0.5" value={settings.terminalFontSize} onChange={(event) => update({terminalFontSize: Number(event.target.value)})}/><b>{settings.terminalFontSize}px</b></span></label></div>
            <div className="settings-section"><h3>Интерфейс</h3><label className="setting-row"><span><strong>Меньше анимаций</strong><small>Отключить плавные переходы</small></span><input className="switch" type="checkbox" checked={settings.reducedMotion} onChange={(event) => update({reducedMotion: event.target.checked})}/></label><label className="setting-row"><span><strong>Подтверждать удаление</strong><small>Для хостов и файлов SFTP</small></span><input className="switch" type="checkbox" checked={settings.confirmDelete} onChange={(event) => update({confirmDelete: event.target.checked})}/></label></div>
        </div>
        <footer className="modal-actions"><button className="primary" onClick={onClose}>Готово</button></footer>
    </Modal>;
}

function HostEditorDialog({host, onClose, onSave}) {
    const fallback = host?.address ? `ssh ${host.port && host.port !== 22 ? `-p ${host.port} ` : ''}${host.user ? `${host.user}@` : ''}${host.address}` : '';
    const [name, setName] = useState(host?.name || '');
    const [command, setCommand] = useState(host?.command || fallback);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const submit = async (event) => {
        event.preventDefault(); setSaving(true); setError('');
        try { await onSave({...host, name: name.trim(), command: command.trim()}); }
        catch (reason) { setError(String(reason).replace(/^Error:\s*/, '')); setSaving(false); }
    };
    return <Modal title={host?.id ? 'Редактировать хост' : 'Добавить хост'} subtitle="Команда сохраняется целиком со всеми SSH-флагами" onClose={saving ? null : onClose} className="host-editor-modal"><form onSubmit={submit}>
        <div className="modal-form"><label><span>Название <small>необязательно</small></span><input autoFocus value={name} onChange={(event) => setName(event.target.value)} placeholder="Production"/></label><label><span>SSH-команда</span><input value={command} onChange={(event) => setCommand(event.target.value)} placeholder="ssh -p 2222 -J jump deploy@example.org" required spellCheck="false"/></label><p className="form-hint">Поддерживаются `-p`, `-l`, `-J`, `-i`, `-o`, туннели и другие аргументы OpenSSH.</p>{error && <p className="form-error">{error}</p>}</div>
        <footer className="modal-actions"><button type="button" onClick={onClose} disabled={saving}>Отмена</button><button className="primary" type="submit" disabled={saving || !command.trim()}>{saving ? 'Сохранение…' : 'Сохранить'}</button></footer>
    </form></Modal>;
}

function SaveHostDialog({request, onResolve}) {
    const connection = request.connection || {};
    return <Modal title="Сохранить новый хост?" subtitle="SSH-авторизация прошла успешно" className="save-host-modal"><div className="save-host-summary"><span className="host-avatar">{String(connection.address || 'SSH').slice(0, 2).toUpperCase()}</span><div><strong>{connection.user ? `${connection.user}@` : ''}{connection.address}</strong><small>Порт {connection.port || 22}</small></div></div><code className="command-preview">{connection.command || `ssh ${connection.target}`}</code><p className="save-host-note">Пароль, если он вводился, будет сохранён в системном хранилище.</p><footer className="modal-actions"><button onClick={() => onResolve(false)}>Не сохранять</button><button className="primary" onClick={() => onResolve(true)}>Сохранить</button></footer></Modal>;
}

function newLocalTab() { return {id: sessionID(), title: 'local', kind: 'local', ready: false}; }

function App() {
    const firstTab = useRef(newLocalTab()).current;
    const terminalRegistry = useRef(new Map());
    const sftpRef = useRef(null);
    const sftpSessionsRef = useRef(new Map());
    const [tabs, setTabs] = useState([firstTab]);
    const [activeTabID, setActiveTabID] = useState(firstTab.id);
    const [hosts, setHosts] = useState([]);
    const [settings, setSettings] = useState(loadSettings);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [hostEditor, setHostEditor] = useState(null);
    const [hostSaveRequest, setHostSaveRequest] = useState(null);
    const [query, setQuery] = useState('');
    const [sidebar, setSidebar] = useState('hosts');
    const [sidebarOpen, setSidebarOpen] = useState(true);
    const [sftp, setSftp] = useState(null);
    const [remotePath, setRemotePath] = useState('.');
    const [remoteFiles, setRemoteFiles] = useState([]);
    const [sftpLoading, setSftpLoading] = useState(false);
    const [sftpError, setSftpError] = useState('');
    const [contextMenu, setContextMenu] = useState(null);
    const [remoteClipboard, setRemoteClipboard] = useState(null);
    const [dropActive, setDropActive] = useState(false);
    const [transferring, setTransferring] = useState(false);
    const [transfer, setTransfer] = useState(null);
    const [transferCancelling, setTransferCancelling] = useState(false);
    const [toast, setToast] = useState('');
    const dragDepth = useRef(0);
    const dropFallbackTimer = useRef(null);
    const transferBusy = useRef(false);

    const activeTab = tabs.find((tab) => tab.id === activeTabID) || tabs[0];
    const refreshHosts = useCallback(() => GetHosts().then((value) => setHosts(value || [])).catch(() => setHosts([])), []);
    const register = useCallback((id, api = undefined) => {
        if (api === undefined) return terminalRegistry.current.get(id);
        if (api) terminalRegistry.current.set(id, api); else terminalRegistry.current.delete(id);
        return api;
    }, []);
    const setReady = useCallback((id, ready) => setTabs((value) => value.map((tab) => tab.id === id ? {...tab, ready} : tab)), []);

    const loadRemote = useCallback(async (connection, path = '.') => {
        if (!connection?.ready) return;
        setSftpLoading(true); setSftpError('');
        try {
            const listing = await ListSFTP(connection.sessionId, path);
            setRemotePath(listing.path || path); setRemoteFiles(listing.files || []);
        } catch (error) { setRemoteFiles([]); setSftpError(String(error).replace(/^Error:\s*/, '')); }
        finally { setSftpLoading(false); }
    }, []);

    const processCommand = useCallback(() => {}, []);

    useEffect(() => { sftpRef.current = sftp; }, [sftp]);

    useEffect(() => {
        document.documentElement.dataset.theme = settings.theme;
        document.documentElement.dataset.reducedMotion = settings.reducedMotion ? 'true' : 'false';
        localStorage.setItem('relay:settings', JSON.stringify(settings));
    }, [settings]);

    useEffect(() => {
        const onKeyDown = (event) => {
            if (event.key !== 'Escape') return;
            if (settingsOpen) setSettingsOpen(false);
            else if (hostEditor) setHostEditor(null);
        };
        window.addEventListener('keydown', onKeyDown);
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [hostEditor, settingsOpen]);

    useEffect(() => {
        refreshHosts();
        EventsOn('hosts:changed', refreshHosts);
        EventsOn('terminal:data', (event) => {
            terminalRegistry.current.get(event.sessionId)?.write(event.data);
        });
        EventsOn('sftp:status', (status) => {
            const session = {...status, sessionId: status.sessionId};
            sftpSessionsRef.current.set(status.sessionId, session);
            setSftp(session); setSidebar('sftp'); setSidebarOpen(true); setRemoteFiles([]);
            if (status.ready) { setSftpError(''); setRemotePath(status.path || '.'); loadRemote(session, status.path || '.'); }
            else setSftpError(status.error || 'Не удалось открыть SFTP');
        });
        EventsOn('sftp:closed', (event) => {
            sftpSessionsRef.current.delete(event.sessionId);
            if (sftpRef.current?.sessionId === event.sessionId) { setSftp(null); setSidebar('hosts'); setRemoteFiles([]); }
        });
        EventsOn('terminal:exit', (event) => {
            terminalRegistry.current.get(event.sessionId)?.exited(); setReady(event.sessionId, false);
            if (sftpRef.current?.sessionId === event.sessionId) { setSftp(null); setSidebar('hosts'); }
        });
        EventsOn('sftp:transfer', (event) => {
            if (sftpRef.current?.sessionId !== event.sessionId) return;
            setTransfer(event); setTransferring(!event.done);
            if (event.done) setTransferCancelling(false);
            if (event.done) setTimeout(() => setTransfer((current) => current?.transferId === event.transferId ? null : current), event.error ? 3500 : 1400);
        });
        EventsOn('host:save-request', (request) => setHostSaveRequest(request));
        return () => EventsOff('hosts:changed', 'terminal:data', 'terminal:exit', 'sftp:status', 'sftp:closed', 'sftp:transfer', 'host:save-request');
    }, [loadRemote, refreshHosts, setReady]);

    useEffect(() => { if (!toast) return; const timer = setTimeout(() => setToast(''), 2600); return () => clearTimeout(timer); }, [toast]);
    const filteredHosts = useMemo(() => { const value = query.toLowerCase(); return (hosts || []).filter((host) => `${host.name} ${host.target} ${host.address} ${host.user} ${host.command}`.toLowerCase().includes(value)); }, [hosts, query]);

    const addLocalTab = () => { const tab = newLocalTab(); setTabs((value) => [...value, tab]); setActiveTabID(tab.id); };
    const toggleSidebar = (section) => {
        if (sidebarOpen && sidebar === section) setSidebarOpen(false);
        else { setSidebar(section); setSidebarOpen(true); }
    };
    const runSSH = (host) => {
        const tab = {id: sessionID(), title: host.name || host.address, kind: 'ssh', hostId: host.id, ready: false};
        setTabs((value) => [...value, tab]); setActiveTabID(tab.id);
    };
    const saveHost = async (host) => {
        const value = await UpsertHost(host);
        setHosts(value || []); setHostEditor(null); setToast(host.id ? 'Хост обновлён' : 'Хост добавлен');
    };
    const resolveHostSave = async (save) => {
        const request = hostSaveRequest;
        setHostSaveRequest(null);
        if (!request) return;
        try {
            const value = await ResolveHostSave(request.sessionId, save);
            setHosts(value || []);
            setToast(save ? 'Хост сохранён' : 'Хост не добавлен');
        } catch (error) { setToast(String(error).replace(/^Error:\s*/, '')); }
    };
    const deleteHost = (host) => {
        if (settings.confirmDelete && !window.confirm(`Удалить хост «${host.name || host.address}»?`)) return;
        DeleteHost(host.id).then((value) => setHosts(value || [])).catch((error) => setToast(String(error)));
    };
    const activateTab = (id) => {
        setActiveTabID(id);
        const remote = sftpSessionsRef.current.get(id);
        if (remote) { setSftp(remote); if (sidebar === 'sftp' && remote.ready) loadRemote(remote, remote.path || '.'); }
    };
    const closeTab = (event, id) => {
        event.stopPropagation();
        const remaining = tabs.filter((tab) => tab.id !== id);
        if (!remaining.length) { const replacement = newLocalTab(); setTabs([replacement]); setActiveTabID(replacement.id); return; }
        setTabs(remaining);
        if (activeTabID === id) setActiveTabID(remaining.at(-1).id);
    };
    const focusActive = () => terminalRegistry.current.get(activeTabID)?.terminal?.focus();
    const copySelection = () => navigator.clipboard.writeText(terminalRegistry.current.get(activeTabID)?.terminal?.getSelection() || '');
    const goRemote = (file) => file.isDir ? loadRemote(sftp, file.path) : DownloadSFTP(sftp.sessionId, file.path).then((path) => path && setToast(`Сохранено: ${path}`)).catch((error) => setToast(String(error)));
    const goUp = () => { if (remotePath === '/' || remotePath === '.') return; const absolute = remotePath.startsWith('/'); const parent = remotePath.replace(/\/+$/, '').split('/').slice(0, -1).join('/'); loadRemote(sftp, parent || (absolute ? '/' : '.')); };
    const afterOperation = async (operation, success) => {
        setContextMenu(null); setTransfer(null); setTransferring(true);
        try { await operation(); setToast(success); await loadRemote(sftpRef.current, remotePath); }
        catch (error) { setToast(String(error).replace(/^Error:\s*/, '')); }
        finally { setTransferring(false); }
    };
    const upload = async () => {
        setContextMenu(null);
        try {
            const paths = await ChooseFilesSFTP();
            if (paths?.length) await uploadPaths(paths);
        } catch (error) { setToast(String(error).replace(/^Error:\s*/, '')); }
    };
    const uploadPaths = useCallback(async (paths) => {
        const selected = [...new Set((paths || []).filter(Boolean))];
        const connection = sftpRef.current;
        if (!selected.length || !connection?.ready || transferBusy.current) return;
        transferBusy.current = true; setContextMenu(null); setTransfer(null); setTransferring(true);
        try {
            await UploadPathsSFTP(connection.sessionId, selected, remotePath);
            setToast(`Загружено: ${fileCount(selected.length)}`);
            await loadRemote(sftpRef.current, remotePath);
        } catch (error) { setToast(String(error).replace(/^Error:\s*/, '')); }
        finally { transferBusy.current = false; setTransferring(false); }
    }, [loadRemote, remotePath]);

    const uploadBrowserFiles = useCallback(async (files) => {
        const selected = [...(files || [])].filter((file) => file?.name);
        const connection = sftpRef.current;
        if (!selected.length || !connection?.ready || transferBusy.current) return;
        transferBusy.current = true; setTransfer(null); setTransferring(true);
        let uploaded = 0;
        try {
            for (const file of selected) {
                const bytes = new Uint8Array(await file.arrayBuffer());
                let binary = '';
                for (let offset = 0; offset < bytes.length; offset += 0x8000) binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000));
                await UploadClipboardFileSFTP(connection.sessionId, remotePath, file.name, btoa(binary));
                uploaded += 1;
            }
            setToast(`Загружено: ${fileCount(uploaded)}`);
            await loadRemote(sftpRef.current, remotePath);
        } catch (error) { setToast(String(error).replace(/^Error:\s*/, '')); }
        finally { transferBusy.current = false; setTransferring(false); }
    }, [loadRemote, remotePath]);
    const cancelTransfer = async () => {
        if (!sftpRef.current?.sessionId || transferCancelling) return;
        setTransferCancelling(true);
        try {
            const stopped = await CancelSFTPTransfer(sftpRef.current.sessionId);
            if (!stopped) setTransferCancelling(false);
        } catch (error) {
            setTransferCancelling(false); setToast(String(error).replace(/^Error:\s*/, ''));
        }
    };
    const createDirectory = () => { const name = window.prompt('Название новой директории'); if (name?.trim()) afterOperation(() => CreateDirectorySFTP(sftp.sessionId, `${remotePath.replace(/\/$/, '')}/${name.trim()}`), 'Директория создана'); };
    const renameRemote = (file) => { const name = window.prompt('Новое имя', file.name); if (name?.trim() && name !== file.name) afterOperation(() => RenameSFTP(sftp.sessionId, file.path, `${remotePath.replace(/\/$/, '')}/${name.trim()}`), 'Объект переименован'); };
    const deleteRemote = (file) => { if (!settings.confirmDelete || window.confirm(`Удалить «${file.name}»${file.isDir ? ' со всем содержимым' : ''}?`)) afterOperation(() => DeleteSFTP(sftp.sessionId, [file.path]), 'Удалено'); };
    const pasteRemote = () => {
        if (!remoteClipboard) return;
        if (remoteClipboard.sessionId !== sftp.sessionId) { setContextMenu(null); setToast('Источник находится на другом SFTP-хосте'); return; }
        const moving = remoteClipboard.move;
        afterOperation(() => CopySFTP(sftp.sessionId, remoteClipboard.paths, remotePath, moving), moving ? 'Перемещено' : 'Скопировано');
        if (moving) setRemoteClipboard(null);
    };

    useEffect(() => {
        if (!sidebarOpen || sidebar !== 'sftp' || !sftp?.ready) return undefined;
        // Wails' CSS target filtering compares native and WebView coordinates.
        // They may differ under Wayland scaling, so scope registration to the
        // visible SFTP panel and accept the native event without that filter.
        OnFileDrop((_x, _y, paths) => {
            if (dropFallbackTimer.current) { clearTimeout(dropFallbackTimer.current); dropFallbackTimer.current = null; }
            dragDepth.current = 0; setDropActive(false); uploadPaths(paths);
        }, false);
        return () => { OnFileDropOff(); if (dropFallbackTimer.current) clearTimeout(dropFallbackTimer.current); };
    }, [sidebar, sidebarOpen, sftp?.ready, uploadPaths]);

    useEffect(() => {
        const closeMenu = () => setContextMenu(null);
        const paste = async (event) => {
            if (!sidebarOpen || sidebar !== 'sftp' || !sftpRef.current?.ready) return;
            const raw = event.clipboardData?.getData('text/uri-list') || event.clipboardData?.getData('x-special/gnome-copied-files') || '';
            const paths = raw.split(/\r?\n/).filter((line) => line.startsWith('file://')).map((line) => decodeURIComponent(line.replace(/^file:\/\//, '')));
            if (paths.length) { event.preventDefault(); uploadPaths(paths); return; }
            const files = [...(event.clipboardData?.files || [])];
            if (!files.length) return;
            event.preventDefault(); await uploadBrowserFiles(files);
        };
        window.addEventListener('click', closeMenu); window.addEventListener('blur', closeMenu); window.addEventListener('paste', paste);
        return () => { window.removeEventListener('click', closeMenu); window.removeEventListener('blur', closeMenu); window.removeEventListener('paste', paste); };
    }, [remotePath, sidebar, sidebarOpen, uploadBrowserFiles, uploadPaths]);

    return <div className="app-shell">
        <header className="titlebar">
            <div className="brand"><span className="brand-mark"><Icon name="terminal" size={15}/></span><span>RELAY</span></div>
            <div className="window-title"><span className={activeTab?.ready ? 'status-dot online' : 'status-dot'}/><span>{activeTab?.title || 'Терминал'}</span></div>
            <div className="window-controls"><button onClick={WindowMinimise} aria-label="Свернуть"><span className="minimise"/></button><button onClick={WindowToggleMaximise} aria-label="Развернуть"><span className="maximise"/></button><button className="close-window" onClick={Quit} aria-label="Закрыть"><Icon name="x" size={14}/></button></div>
        </header>
        <main className={`workspace ${sidebarOpen ? '' : 'sidebar-hidden'}`}>
            <nav className="activity-bar" aria-label="Разделы"><button className={sidebarOpen && sidebar === 'hosts' ? 'active' : ''} onClick={() => toggleSidebar('hosts')} title={sidebarOpen && sidebar === 'hosts' ? 'Скрыть SSH-хосты' : 'SSH-хосты'}><Icon name="server" size={20}/></button>{sftp && <button className={sidebarOpen && sidebar === 'sftp' ? 'active' : ''} onClick={() => toggleSidebar('sftp')} title={sidebarOpen && sidebar === 'sftp' ? 'Скрыть SFTP' : 'SFTP'}><Icon name="folderOpen" size={20}/><span className="activity-badge"/></button>}<div className="activity-spacer"/><button className={settingsOpen ? 'active settings-button' : 'settings-button'} onClick={() => setSettingsOpen(true)} title="Настройки"><Icon name="settings" size={19}/></button></nav>
            <aside className="sidebar">
                {sidebar === 'hosts' ? <>
                    <div className="sidebar-heading"><div><span className="eyebrow">ПОДКЛЮЧЕНИЯ</span><h1>SSH-хосты</h1></div><span className="heading-actions"><button onClick={() => setHostEditor({})} title="Добавить хост"><Icon name="plus" size={14}/></button><span className="counter">{hosts.length}</span></span></div>
                    <label className="search-box"><Icon name="search" size={16}/><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Найти хост…"/></label>
                    <div className="host-list">{filteredHosts.length === 0 ? <div className="empty-state"><span className="empty-icon"><Icon name="server" size={22}/></span><strong>{hosts.length ? 'Ничего не найдено' : 'Здесь появятся хосты'}</strong><p>{hosts.length ? 'Попробуйте изменить запрос' : 'Добавьте хост кнопкой + или сохраните его после успешного SSH-входа'}</p></div> : filteredHosts.map((host) => <div className="host-card" key={host.id}>
                        <button className="host-main" onClick={() => runSSH(host)}><span className="host-avatar">{host.address.slice(0, 2).toUpperCase()}</span><span className="host-copy"><strong>{host.name || host.address}</strong><small>{host.name ? `${host.user ? `${host.user}@` : ''}${host.address} · ` : `${host.user || 'текущий пользователь'} · `}:{host.port}{host.hasPassword ? ' · пароль сохранён' : ''}</small></span>{host.hasPassword && <span className="password-mark" title="Пароль хранится в системном хранилище"><Icon name="lock" size={12}/></span>}<Icon name="chevron" size={15}/></button>
                        <div className="host-meta"><span>{host.lastConnected ? relativeTime(host.lastConnected) : 'ещё не подключались'}</span><span className="host-actions"><button onClick={() => setHostEditor(host)} title="Редактировать"><Icon name="edit" size={13}/></button><button onClick={() => ToggleFavorite(host.id).then((value) => setHosts(value || []))} title="В избранное" className={host.favorite ? 'favorite' : ''}><Icon name="star" size={14} filled={host.favorite}/></button><button onClick={() => deleteHost(host)} title="Удалить"><Icon name="trash" size={14}/></button></span></div>
                    </div>)}</div>
                </> : <>
                    <div className="sidebar-heading sftp-heading"><div><span className="eyebrow">SFTP</span><h1>{sftp?.address}</h1></div><span className={`connection-pill ${sftp?.ready ? '' : 'failed'}`}><span/>{sftp?.ready ? 'активно' : 'ошибка'}</span></div>
                    <div className="sftp-toolbar"><button onClick={goUp} disabled={!sftp?.ready || remotePath === '.' || remotePath === '/'}>..</button><button className="path" title={remotePath}>{remotePath}</button><button disabled={!sftp?.ready} onClick={() => loadRemote(sftp, remotePath)}><Icon name="refresh" size={15}/></button><button disabled={!sftp?.ready || transferring} onClick={createDirectory} title="Новая директория"><Icon name="plus" size={15}/></button><button disabled={!sftp?.ready || transferring} onClick={upload} title="Загрузить несколько файлов"><Icon name="upload" size={15}/></button></div>
                    <div className={`remote-list drop-zone ${dropActive ? 'drop-active' : ''}`} onContextMenu={(event) => { event.preventDefault(); setContextMenu({x: event.clientX, y: event.clientY, file: null}); }} onDragEnter={(event) => { if (Array.from(event.dataTransfer?.types || []).includes('Files')) { event.preventDefault(); dragDepth.current += 1; setDropActive(true); } }} onDragOver={(event) => { event.preventDefault(); if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'; }} onDragLeave={() => { dragDepth.current = Math.max(0, dragDepth.current - 1); if (!dragDepth.current) setDropActive(false); }} onDrop={(event) => { event.preventDefault(); dragDepth.current = 0; setDropActive(false); const files = [...(event.dataTransfer?.files || [])]; if (files.length) { if (dropFallbackTimer.current) clearTimeout(dropFallbackTimer.current); dropFallbackTimer.current = setTimeout(() => { dropFallbackTimer.current = null; uploadBrowserFiles(files); }, 180); } }}>
                        {sftpLoading ? <div className="loading"><span/><span/><span/><small>Загрузка…</small></div> : transferring && !transfer ? <div className="loading"><span/><span/><span/><small>Подготовка передачи…</small></div> : sftpError ? <div className="empty-state error-state"><span className="empty-icon"><Icon name="folder" size={22}/></span><strong>Нет доступа к SFTP</strong><p>{sftpError}</p><small>SSH-терминал продолжает работать. Проверьте поддержку SFTP на сервере.</small></div> : remoteFiles.length === 0 ? <div className="empty-state"><span className="empty-icon"><Icon name="folder" size={22}/></span><strong>Папка пуста</strong><p>Перетащите или вставьте сюда файлы</p></div> : remoteFiles.map((file) => <button className="remote-file" key={file.path} onDoubleClick={() => goRemote(file)} onContextMenu={(event) => { event.preventDefault(); event.stopPropagation(); setContextMenu({x: event.clientX, y: event.clientY, file}); }}><span className={file.isDir ? 'file-icon folder' : 'file-icon'}><Icon name={file.isDir ? 'folder' : 'file'} size={17}/></span><span className="file-name">{file.name}</span><small>{file.isDir ? '' : formatSize(file.size)}</small>{!file.isDir && <span className="download-icon" onClick={(event) => { event.stopPropagation(); goRemote(file); }}><Icon name="download" size={14}/></span>}</button>)}
                        {dropActive && <div className="drop-overlay"><Icon name="upload" size={25}/><strong>Загрузить в {remotePath}</strong><span>Файлы и директории</span></div>}
                        {transfer && <TransferCard transfer={transfer} cancelling={transferCancelling} onCancel={cancelTransfer}/>} 
                    </div>
                    <div className="sftp-footer"><span><span className={sftp?.ready ? 'status-dot online' : 'status-dot'}/> {sftp?.target}</span><button onClick={() => setSidebarOpen(false)}>Скрыть</button></div>
                </>}
            </aside>
            <section className="terminal-pane" onClick={focusActive}>
                <div className="tab-strip">{tabs.map((tab) => <button key={tab.id} className={`terminal-tab ${tab.id === activeTabID ? 'active' : ''}`} onClick={() => activateTab(tab.id)}><Icon name={tab.kind === 'ssh' ? 'server' : 'terminal'} size={14}/><span>{tab.title}</span><span className={tab.ready ? 'tab-dot online' : 'tab-dot'}/><span className="tab-close" onClick={(event) => closeTab(event, tab.id)}><Icon name="x" size={11}/></span></button>)}<button className="new-tab" onClick={addLocalTab} title="Новый локальный терминал"><Icon name="plus" size={15}/></button><div className="tab-spacer"/><button className="copy-button" onClick={(event) => { event.stopPropagation(); copySelection(); }} title="Копировать"><Icon name="copy" size={14}/></button></div>
                <div className="terminal-stack">{tabs.map((tab) => <TerminalView key={tab.id} tab={tab} active={tab.id === activeTabID} register={register} onCommand={processCommand} onReady={setReady} appearance={settings}/>)}</div>
                <footer className="statusbar"><span><span className={activeTab?.ready ? 'status-dot online' : 'status-dot'}/>{activeTab?.ready ? 'shell запущен' : 'shell остановлен'}</span><span>{tabs.length} вклад.</span><span>UTF-8</span><span>xterm-256color</span></footer>
            </section>
        </main>
        {contextMenu && <div className="context-menu" style={{left: Math.min(contextMenu.x, window.innerWidth - 190), top: Math.min(contextMenu.y, window.innerHeight - 285)}} onClick={(event) => event.stopPropagation()}>
            {contextMenu.file && <><button onClick={() => renameRemote(contextMenu.file)}>Переименовать</button><button onClick={() => { setRemoteClipboard({sessionId: sftp.sessionId, paths: [contextMenu.file.path], move: false}); setContextMenu(null); setToast('Скопировано в буфер SFTP'); }}>Копировать</button><button onClick={() => { setRemoteClipboard({sessionId: sftp.sessionId, paths: [contextMenu.file.path], move: true}); setContextMenu(null); setToast('Выбрано для перемещения'); }}>Переместить</button><button onClick={() => goRemote(contextMenu.file)}>Скачать</button><span className="menu-separator"/><button className="danger" onClick={() => deleteRemote(contextMenu.file)}>Удалить</button><span className="menu-separator"/></>}
            <button onClick={createDirectory}>Новая директория</button><button onClick={upload}>Загрузить файлы…</button>{remoteClipboard && <button onClick={pasteRemote}>Вставить {remoteClipboard.move ? '(переместить)' : '(копировать)'}</button>}<button onClick={() => loadRemote(sftp, remotePath)}>Обновить</button>
        </div>}
        {settingsOpen && <SettingsDialog settings={settings} onChange={setSettings} onClose={() => setSettingsOpen(false)}/>}
        {hostEditor && <HostEditorDialog key={hostEditor.id || 'new'} host={hostEditor} onClose={() => setHostEditor(null)} onSave={saveHost}/>}
        {hostSaveRequest && <SaveHostDialog request={hostSaveRequest} onResolve={resolveHostSave}/>}
        {toast && <div className="toast">{toast}</div>}
    </div>;
}

export default function Root() { return <AppErrorBoundary><App/></AppErrorBoundary>; }
