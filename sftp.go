package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pkgsftp "github.com/pkg/sftp"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type remoteSession struct {
	ssh        *ssh.Client
	client     *pkgsftp.Client
	connection ConnectionCommand
}

type RemoteFile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"isDir"`
	ModTime string `json:"modTime"`
}

type SFTPListing struct {
	Path  string       `json:"path"`
	Files []RemoteFile `json:"files"`
}

type SFTPStatusEvent struct {
	SessionID string `json:"sessionId"`
	Target    string `json:"target"`
	Address   string `json:"address"`
	Ready     bool   `json:"ready"`
	Path      string `json:"path"`
	Error     string `json:"error"`
}

type SFTPTransferEvent struct {
	SessionID  string `json:"sessionId"`
	TransferID string `json:"transferId"`
	Direction  string `json:"direction"`
	Name       string `json:"name"`
	FilesDone  int    `json:"filesDone"`
	FilesTotal int    `json:"filesTotal"`
	BytesDone  int64  `json:"bytesDone"`
	BytesTotal int64  `json:"bytesTotal"`
	StartedAt  int64  `json:"startedAt"`
	ElapsedMS  int64  `json:"elapsedMs"`
	Done       bool   `json:"done"`
	Error      string `json:"error"`
}

type transferReporter struct {
	app      *App
	event    SFTPTransferEvent
	started  time.Time
	lastEmit time.Time
}

type transferReader struct {
	reader   io.Reader
	reporter *transferReporter
	control  *transferControl
}

func (r *transferReader) Read(buffer []byte) (int, error) {
	if r.control.cancelled() {
		return 0, errTransferCancelled
	}
	read, err := r.reader.Read(buffer)
	if read > 0 {
		r.reporter.addBytes(int64(read))
	}
	if r.control.cancelled() {
		return read, errTransferCancelled
	}
	return read, err
}

var errTransferCancelled = errors.New("передача отменена")

type transferControl struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closers map[io.Closer]struct{}
}

func newTransferControl() *transferControl {
	ctx, cancel := context.WithCancel(context.Background())
	return &transferControl{ctx: ctx, cancel: cancel, closers: make(map[io.Closer]struct{})}
}

func (c *transferControl) cancelled() bool {
	return c.ctx.Err() != nil
}

func (c *transferControl) add(closers ...io.Closer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, closer := range closers {
		if closer != nil {
			c.closers[closer] = struct{}{}
		}
	}
}

func (c *transferControl) remove(closers ...io.Closer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, closer := range closers {
		delete(c.closers, closer)
	}
}

func (c *transferControl) stop() {
	c.cancel()
	c.mu.Lock()
	closers := make([]io.Closer, 0, len(c.closers))
	for closer := range c.closers {
		closers = append(closers, closer)
	}
	c.mu.Unlock()
	for _, closer := range closers {
		_ = closer.Close()
	}
}

func newTransferReporter(app *App, sessionID, direction string, filesTotal int, bytesTotal int64) *transferReporter {
	started := time.Now()
	reporter := &transferReporter{app: app, started: started, event: SFTPTransferEvent{
		SessionID: sessionID, TransferID: fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano()),
		Direction: direction, FilesTotal: filesTotal, BytesTotal: bytesTotal, StartedAt: started.UnixMilli(),
	}}
	reporter.emit(true)
	return reporter
}

func (r *transferReporter) current(name string) {
	r.event.Name = name
	r.emit(true)
}

func (r *transferReporter) addBytes(count int64) {
	r.event.BytesDone += count
	r.emit(false)
}

func (r *transferReporter) fileDone() {
	r.event.FilesDone++
	r.emit(true)
}

func (r *transferReporter) finish(err error) {
	r.event.Done = true
	if err != nil {
		r.event.Error = err.Error()
	} else {
		r.event.FilesDone = r.event.FilesTotal
		r.event.BytesDone = r.event.BytesTotal
	}
	r.emit(true)
}

func (r *transferReporter) emit(force bool) {
	if r.app.ctx == nil || (!force && time.Since(r.lastEmit) < 80*time.Millisecond) {
		return
	}
	r.event.ElapsedMS = time.Since(r.started).Milliseconds()
	r.lastEmit = time.Now()
	wailsruntime.EventsEmit(r.app.ctx, "sftp:transfer", r.event)
}

func (a *App) beginTransfer(sessionID string) (*transferControl, error) {
	a.transferMu.Lock()
	defer a.transferMu.Unlock()
	if _, exists := a.transfers[sessionID]; exists {
		return nil, errors.New("другая передача уже выполняется")
	}
	control := newTransferControl()
	a.transfers[sessionID] = control
	return control, nil
}

func (a *App) endTransfer(sessionID string, control *transferControl) {
	a.transferMu.Lock()
	if a.transfers[sessionID] == control {
		delete(a.transfers, sessionID)
	}
	a.transferMu.Unlock()
}

// CancelSFTPTransfer stops the active upload or download for this SFTP session.
func (a *App) CancelSFTPTransfer(sessionID string) bool {
	a.transferMu.Lock()
	control := a.transfers[sessionID]
	a.transferMu.Unlock()
	if control == nil {
		return false
	}
	control.stop()
	return true
}

func (a *App) connectSFTP(sessionID string, connection ConnectionCommand, password string) {
	if password == "" {
		password = a.getCredential(hostID(connection))
	}
	sshClient, sftpClient, err := dialSFTP(connection, password)
	if err != nil {
		a.emitSFTPStatus(SFTPStatusEvent{SessionID: sessionID, Target: connection.Target, Address: connection.Address, Error: err.Error()})
		return
	}
	home, err := sftpClient.Getwd()
	if err != nil || home == "" {
		home = "."
	}
	session := &remoteSession{ssh: sshClient, client: sftpClient, connection: connection}
	a.sftpMu.Lock()
	old := a.sftp[sessionID]
	a.sftp[sessionID] = session
	a.sftpMu.Unlock()
	if old != nil {
		_ = old.client.Close()
		_ = old.ssh.Close()
	}
	a.emitSFTPStatus(SFTPStatusEvent{SessionID: sessionID, Target: connection.Target, Address: connection.Address, Ready: true, Path: home})
}

func dialSFTP(connection ConnectionCommand, password string) (*ssh.Client, *pkgsftp.Client, error) {
	username := connection.User
	if username == "" {
		if current, err := user.Current(); err == nil {
			username = current.Username
		}
	}
	authMethods, agentConn := sshAuthMethods(password)
	if agentConn != nil {
		defer agentConn.Close()
	}
	if len(authMethods) == 0 {
		return nil, nil, errors.New("нет доступного пароля или SSH-ключа")
	}
	// The interactive OpenSSH session has already authenticated this endpoint.
	// Do not perform a second known_hosts check for the auxiliary SFTP channel:
	// it would reject hosts whose stored key is stale even after the user has
	// explicitly accepted the current key in the terminal.
	config := &ssh.ClientConfig{User: username, Auth: authMethods, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 10 * time.Second}
	address := net.JoinHostPort(connection.Address, strconv.Itoa(connection.Port))
	sshClient, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось открыть SFTP: %w", err)
	}
	sftpClient, err := pkgsftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("SFTP недоступен: %w", err)
	}
	return sshClient, sftpClient, nil
}

func sshAuthMethods(password string) ([]ssh.AuthMethod, net.Conn) {
	methods := make([]ssh.AuthMethod, 0, 3)
	if password != "" {
		methods = append(methods, ssh.Password(password), ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for index := range answers {
				answers[index] = password
			}
			return answers, nil
		}))
	}
	var agentConn net.Conn
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		if connection, err := net.Dial("unix", socket); err == nil {
			agentConn = connection
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(connection).Signers))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			data, err := os.ReadFile(filepath.Join(home, ".ssh", name))
			if err != nil {
				continue
			}
			if signer, err := ssh.ParsePrivateKey(data); err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}
	return methods, agentConn
}

func (a *App) emitSFTPStatus(status SFTPStatusEvent) {
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "sftp:status", status)
	}
}

func (a *App) getSFTP(sessionID string) (*remoteSession, error) {
	a.sftpMu.Lock()
	defer a.sftpMu.Unlock()
	session := a.sftp[sessionID]
	if session == nil {
		return nil, errors.New("SFTP-соединение не открыто")
	}
	return session, nil
}

func (a *App) closeSFTP(sessionID string) {
	a.CancelSFTPTransfer(sessionID)
	a.sftpMu.Lock()
	session := a.sftp[sessionID]
	delete(a.sftp, sessionID)
	a.sftpMu.Unlock()
	if session != nil {
		_ = session.client.Close()
		_ = session.ssh.Close()
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "sftp:closed", map[string]string{"sessionId": sessionID})
	}
}

func (a *App) closeAllSFTP() {
	a.transferMu.Lock()
	controls := make([]*transferControl, 0, len(a.transfers))
	for _, control := range a.transfers {
		controls = append(controls, control)
	}
	a.transferMu.Unlock()
	for _, control := range controls {
		control.stop()
	}
	a.sftpMu.Lock()
	sessions := a.sftp
	a.sftp = make(map[string]*remoteSession)
	a.sftpMu.Unlock()
	for _, session := range sessions {
		_ = session.client.Close()
		_ = session.ssh.Close()
	}
}

func (a *App) ListSFTP(sessionID, remotePath string) (SFTPListing, error) {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return SFTPListing{Files: []RemoteFile{}}, err
	}
	if remotePath == "" {
		remotePath = "."
	}
	entries, err := session.client.ReadDir(remotePath)
	if err != nil {
		return SFTPListing{Path: remotePath, Files: []RemoteFile{}}, err
	}
	files := make([]RemoteFile, 0, len(entries))
	for _, entry := range entries {
		files = append(files, RemoteFile{Name: entry.Name(), Path: pathpkg.Join(remotePath, entry.Name()), Size: entry.Size(), Mode: entry.Mode().String(), IsDir: entry.IsDir(), ModTime: entry.ModTime().Format(time.RFC3339)})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})
	return SFTPListing{Path: remotePath, Files: files}, nil
}

func (a *App) CreateDirectorySFTP(sessionID, remotePath string) error {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	return session.client.MkdirAll(remotePath)
}

func (a *App) RenameSFTP(sessionID, oldPath, newPath string) error {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	return session.client.Rename(oldPath, newPath)
}

func (a *App) DeleteSFTP(sessionID string, paths []string) error {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	for _, remotePath := range paths {
		if err := removeRemote(session.client, remotePath); err != nil {
			return err
		}
	}
	return nil
}

func removeRemote(client *pkgsftp.Client, remotePath string) error {
	info, err := client.Lstat(remotePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return client.Remove(remotePath)
	}
	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := removeRemote(client, pathpkg.Join(remotePath, entry.Name())); err != nil {
			return err
		}
	}
	return client.RemoveDirectory(remotePath)
}

func (a *App) CopySFTP(sessionID string, paths []string, destination string, move bool) error {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	for _, source := range paths {
		target := pathpkg.Join(destination, pathpkg.Base(source))
		if move {
			if err := session.client.Rename(source, target); err == nil {
				continue
			}
		}
		if err := copyRemote(session.client, source, target); err != nil {
			return err
		}
		if move {
			if err := removeRemote(session.client, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyRemote(client *pkgsftp.Client, source, target string) error {
	info, err := client.Stat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if target == source || strings.HasPrefix(target+"/", strings.TrimSuffix(source, "/")+"/") {
			return errors.New("нельзя скопировать директорию внутрь самой себя")
		}
		if err := client.MkdirAll(target); err != nil {
			return err
		}
		entries, err := client.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyRemote(client, pathpkg.Join(source, entry.Name()), pathpkg.Join(target, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	input, err := client.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := client.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (a *App) ChooseAndUploadSFTP(sessionID, remoteDir string) (int, error) {
	paths, err := a.ChooseFilesSFTP()
	if err != nil || len(paths) == 0 {
		return 0, err
	}
	if err := a.UploadPathsSFTP(sessionID, paths, remoteDir); err != nil {
		return 0, err
	}
	return len(paths), nil
}

// ChooseFilesSFTP only opens the native multi-select dialog. Keeping selection
// separate from uploading lets the frontend preserve the complete selection and
// show transfer state before the first byte is sent.
func (a *App) ChooseFilesSFTP() ([]string, error) {
	if a.ctx == nil {
		return nil, errors.New("application is not ready")
	}
	paths, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Выберите один или несколько файлов (Ctrl/Shift)"})
	if err != nil || len(paths) == 0 {
		return []string{}, err
	}
	return paths, nil
}

func (a *App) UploadPathsSFTP(sessionID string, localPaths []string, remoteDir string) error {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	filesTotal, bytesTotal, err := localTransferSize(localPaths)
	if err != nil {
		return err
	}
	control, err := a.beginTransfer(sessionID)
	if err != nil {
		return err
	}
	defer a.endTransfer(sessionID, control)
	reporter := newTransferReporter(a, sessionID, "upload", filesTotal, bytesTotal)
	for _, localPath := range localPaths {
		if err := uploadLocal(session.client, localPath, pathpkg.Join(remoteDir, filepath.Base(localPath)), reporter, control); err != nil {
			reporter.finish(err)
			return err
		}
	}
	reporter.finish(nil)
	return nil
}

func localTransferSize(paths []string) (int, int64, error) {
	files := 0
	var size int64
	for _, localPath := range paths {
		pathFiles, pathSize, err := localPathTransferSize(localPath)
		if err != nil {
			return 0, 0, err
		}
		files += pathFiles
		size += pathSize
	}
	return files, size, nil
}

func localPathTransferSize(localPath string) (int, int64, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 1, info.Size(), nil
	}
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return 0, 0, err
	}
	files := 0
	var size int64
	for _, entry := range entries {
		childFiles, childSize, err := localPathTransferSize(filepath.Join(localPath, entry.Name()))
		if err != nil {
			return 0, 0, err
		}
		files += childFiles
		size += childSize
	}
	return files, size, nil
}

func uploadLocal(client *pkgsftp.Client, localPath, remotePath string, reporter *transferReporter, control *transferControl) error {
	if control.cancelled() {
		return errTransferCancelled
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := client.MkdirAll(remotePath); err != nil {
			return err
		}
		entries, err := os.ReadDir(localPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := uploadLocal(client, filepath.Join(localPath, entry.Name()), pathpkg.Join(remotePath, entry.Name()), reporter, control); err != nil {
				return err
			}
		}
		return nil
	}
	input, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer input.Close()
	reporter.current(filepath.Base(localPath))
	output, err := client.Create(remotePath)
	if err != nil {
		return err
	}
	control.add(input, output)
	defer control.remove(input, output)
	_, copyErr := io.Copy(output, &transferReader{reader: input, reporter: reporter, control: control})
	closeErr := output.Close()
	if control.cancelled() {
		return errTransferCancelled
	}
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	reporter.fileDone()
	return nil
}

func (a *App) UploadClipboardFileSFTP(sessionID, remoteDir, name, encoded string) error {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return err
	}
	control, err := a.beginTransfer(sessionID)
	if err != nil {
		return err
	}
	defer a.endTransfer(sessionID, control)
	reporter := newTransferReporter(a, sessionID, "upload", 1, int64(len(data)))
	reporter.current(name)
	file, err := session.client.Create(pathpkg.Join(remoteDir, pathpkg.Base(name)))
	if err != nil {
		reporter.finish(err)
		return err
	}
	control.add(file)
	defer control.remove(file)
	_, writeErr := io.Copy(file, &transferReader{reader: bytes.NewReader(data), reporter: reporter, control: control})
	closeErr := file.Close()
	if control.cancelled() {
		reporter.finish(errTransferCancelled)
		return errTransferCancelled
	}
	if writeErr != nil {
		reporter.finish(writeErr)
		return writeErr
	}
	if closeErr != nil {
		reporter.finish(closeErr)
		return closeErr
	}
	reporter.fileDone()
	reporter.finish(nil)
	return nil
}

func (a *App) DownloadSFTP(sessionID, remotePath string) (string, error) {
	session, err := a.getSFTP(sessionID)
	if err != nil {
		return "", err
	}
	info, err := session.client.Stat(remotePath)
	if err != nil {
		return "", err
	}
	var destination string
	if info.IsDir() {
		root, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Выбрать папку для скачивания"})
		if err != nil || root == "" {
			return "", err
		}
		destination = filepath.Join(root, pathpkg.Base(remotePath))
	} else {
		destination, err = wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{DefaultFilename: pathpkg.Base(remotePath), Title: "Сохранить удалённый файл"})
		if err != nil || destination == "" {
			return "", err
		}
	}
	filesTotal, bytesTotal, err := remoteTransferSize(session.client, remotePath)
	if err != nil {
		return "", err
	}
	control, err := a.beginTransfer(sessionID)
	if err != nil {
		return "", err
	}
	defer a.endTransfer(sessionID, control)
	reporter := newTransferReporter(a, sessionID, "download", filesTotal, bytesTotal)
	if err := downloadRemote(session.client, remotePath, destination, reporter, control); err != nil {
		reporter.finish(err)
		return "", err
	}
	reporter.finish(nil)
	return destination, nil
}

func remoteTransferSize(client *pkgsftp.Client, remotePath string) (int, int64, error) {
	info, err := client.Stat(remotePath)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 1, info.Size(), nil
	}
	entries, err := client.ReadDir(remotePath)
	if err != nil {
		return 0, 0, err
	}
	files := 0
	var size int64
	for _, entry := range entries {
		childFiles, childSize, err := remoteTransferSize(client, pathpkg.Join(remotePath, entry.Name()))
		if err != nil {
			return 0, 0, err
		}
		files += childFiles
		size += childSize
	}
	return files, size, nil
}

func downloadRemote(client *pkgsftp.Client, remotePath, localPath string, reporter *transferReporter, control *transferControl) error {
	if control.cancelled() {
		return errTransferCancelled
	}
	info, err := client.Stat(remotePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(localPath, 0o755); err != nil {
			return err
		}
		entries, err := client.ReadDir(remotePath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := downloadRemote(client, pathpkg.Join(remotePath, entry.Name()), filepath.Join(localPath, entry.Name()), reporter, control); err != nil {
				return err
			}
		}
		return nil
	}
	input, err := client.Open(remotePath)
	if err != nil {
		return err
	}
	defer input.Close()
	reporter.current(pathpkg.Base(remotePath))
	output, err := os.Create(localPath)
	if err != nil {
		return err
	}
	control.add(input, output)
	defer control.remove(input, output)
	_, copyErr := io.Copy(output, &transferReader{reader: input, reporter: reporter, control: control})
	closeErr := output.Close()
	if control.cancelled() {
		return errTransferCancelled
	}
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	reporter.fileDone()
	return nil
}
