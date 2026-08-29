package session

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/gin-gonic/gin"
)

type startWorkbenchCommandRequest struct {
	Command    string `json:"command" binding:"required"`
	WorkDir    string `json:"work_dir"`
	TimeoutSec int    `json:"timeout_sec"`
}

func (h *Handler) StartWorkbenchCommand(c *gin.Context) {
	if h.workbench == nil {
		c.Error(apperrors.NewInternalServerError("workbench unavailable"))
		return
	}
	var req startWorkbenchCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	cmd, err := h.workbench.StartCommand(c.Request.Context(), paramSessionID(c), req.Command,
		req.WorkDir, time.Duration(req.TimeoutSec)*time.Second)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": cmd})
}

func (h *Handler) StreamWorkbenchCommand(c *gin.Context) {
	if h.workbench == nil {
		c.Error(apperrors.NewInternalServerError("workbench unavailable"))
		return
	}
	cmd, err := h.workbench.Command(c.Request.Context(), paramSessionID(c), c.Param("command_id"))
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	events, unsubscribe := cmd.Subscribe()
	defer unsubscribe()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Error(apperrors.NewInternalServerError("streaming unavailable"))
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			payload, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, payload)
			flusher.Flush()
			if event.Type == "complete" {
				return
			}
		}
	}
}

func (h *Handler) CancelWorkbenchCommand(c *gin.Context) {
	if h.workbench == nil {
		c.Error(apperrors.NewInternalServerError("workbench unavailable"))
		return
	}
	if err := h.workbench.CancelCommand(c.Request.Context(), paramSessionID(c), c.Param("command_id")); err != nil {
		c.Error(workbenchError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) ListWorkbenchFiles(c *gin.Context) {
	store, sessionID, err := h.workbenchFileStore(c)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	dir, err := service.CleanWorkbenchOutputPath(c.Query("path"), true)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	var entries []sandbox.RemoteDirEntry
	if lister, ok := store.(sandbox.SessionDirectoryLister); ok {
		entries, err = lister.ListSessionDirectory(c.Request.Context(), sessionID, dir)
	} else {
		entries, err = store.ListSessionFiles(c.Request.Context(), sessionID, dir)
	}
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	result := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		if e.Path == path.Join(service.WorkbenchOutputRoot, ".weknora-terminal") ||
			strings.HasPrefix(e.Path, path.Join(service.WorkbenchOutputRoot, ".weknora-terminal")+"/") {
			continue
		}
		clean, cleanErr := service.CleanWorkbenchOutputPath(e.Path, true)
		if cleanErr != nil || (clean != dir && !strings.HasPrefix(clean, dir+"/")) {
			continue
		}
		result = append(result, gin.H{"name": e.Name, "path": clean, "is_dir": e.Type == sandbox.RemoteEntryDir,
			"size": e.Size, "mod_time": e.ModTime})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"path": dir, "entries": result}})
}

func (h *Handler) UploadWorkbenchFile(c *gin.Context) {
	store, sessionID, err := h.workbenchFileStore(c)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		c.Error(apperrors.NewBadRequestError("file is required"))
		return
	}
	if file.Size > 50*1024*1024 {
		c.Error(apperrors.NewBadRequestError("file exceeds 50 MiB"))
		return
	}
	dir, err := service.CleanWorkbenchOutputPath(c.PostForm("path"), true)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	target, err := service.CleanWorkbenchOutputPath(path.Join(dir, path.Base(strings.ReplaceAll(file.Filename, "\\", "/"))), false)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	r, err := file.Open()
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, 50*1024*1024+1))
	if err != nil || len(data) > 50*1024*1024 {
		c.Error(apperrors.NewBadRequestError("invalid upload"))
		return
	}
	outputStore, ok := store.(sandbox.SessionOutputFileStore)
	if !ok {
		c.Error(apperrors.NewInternalServerError("sandbox output is read-only"))
		return
	}
	if err := outputStore.WriteSessionOutputFile(c.Request.Context(), sessionID, target, data); err != nil {
		c.Error(workbenchError(err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"path": target}})
}

func (h *Handler) DownloadWorkbenchFile(c *gin.Context) {
	store, sessionID, err := h.workbenchFileStore(c)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	target, err := service.CleanWorkbenchOutputPath(c.Query("path"), false)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	stat, err := store.StatSessionFile(c.Request.Context(), sessionID, target)
	if err != nil || stat == nil || stat.Type != sandbox.RemoteEntryFile {
		c.Error(apperrors.NewNotFoundError("file not found"))
		return
	}
	data, err := store.ReadSessionFile(c.Request.Context(), sessionID, target)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	name := path.Base(target)
	ct := mime.TypeByExtension(strings.ToLower(path.Ext(name)))
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Header("Content-Type", ct)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", buildAttachmentHeader(name))
	c.Header("Content-Length", strconv.Itoa(len(data)))
	c.Data(http.StatusOK, ct, data)
}

type renameWorkbenchFileRequest struct {
	Path    string `json:"path"`
	NewName string `json:"new_name"`
}

func (h *Handler) RenameWorkbenchFile(c *gin.Context) {
	store, sessionID, err := h.workbenchFileStore(c)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	var req renameWorkbenchFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	source, err := service.CleanWorkbenchOutputPath(req.Path, false)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(req.NewName), "\\", "/"))
	if name == "." || name == "" || name != req.NewName {
		c.Error(apperrors.NewBadRequestError("new_name must be a file name"))
		return
	}
	target, err := service.CleanWorkbenchOutputPath(path.Join(path.Dir(source), name), false)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	stat, err := store.StatSessionFile(c.Request.Context(), sessionID, source)
	if err != nil || stat == nil || stat.Type != sandbox.RemoteEntryFile {
		c.Error(apperrors.NewBadRequestError("only files can be renamed"))
		return
	}
	data, err := store.ReadSessionFile(c.Request.Context(), sessionID, source)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	outputStore, ok := store.(sandbox.SessionOutputFileStore)
	if !ok {
		c.Error(apperrors.NewInternalServerError("sandbox output is read-only"))
		return
	}
	if err = outputStore.WriteSessionOutputFile(c.Request.Context(), sessionID, target, data); err == nil {
		err = outputStore.RemoveSessionOutputPath(c.Request.Context(), sessionID, source)
	}
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"path": target}})
}

func (h *Handler) DeleteWorkbenchFile(c *gin.Context) {
	store, sessionID, err := h.workbenchFileStore(c)
	if err != nil {
		c.Error(workbenchError(err))
		return
	}
	target, err := service.CleanWorkbenchOutputPath(c.Query("path"), false)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	outputStore, ok := store.(sandbox.SessionOutputFileStore)
	if !ok {
		c.Error(apperrors.NewInternalServerError("sandbox output is read-only"))
		return
	}
	if err := outputStore.RemoveSessionOutputPath(c.Request.Context(), sessionID, target); err != nil {
		c.Error(workbenchError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) workbenchFileStore(c *gin.Context) (sandbox.SessionFileStore, string, error) {
	if h.workbench == nil {
		return nil, "", stderrors.New("workbench unavailable")
	}
	sessionID := paramSessionID(c)
	store, err := h.workbench.FileStore(c.Request.Context(), sessionID)
	return store, sessionID, err
}

func workbenchError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "not found") {
		return apperrors.NewNotFoundError(msg)
	}
	if strings.Contains(msg, "outside") || strings.Contains(msg, "required") || strings.Contains(msg, "must stay") {
		return apperrors.NewBadRequestError(msg)
	}
	return apperrors.NewInternalServerError(msg)
}
