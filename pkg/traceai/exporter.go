package traceai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

type Exporter interface {
	Export(event models.ToolEvent) error
	Close() error
}

type LocalExporter struct {
	mu   sync.Mutex
	file *os.File
}

func NewLocalExporter() *LocalExporter {
	return &LocalExporter{}
}

func (e *LocalExporter) Export(event models.ToolEvent) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file == nil {
		name := filepath.Join(os.TempDir(), "traceai-events.jsonl")
		file, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		e.file = file
	}
	payload, err := json.Marshal(event.Normalize())
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(e.file, string(payload))
	return err
}

func (e *LocalExporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file == nil {
		return nil
	}
	err := e.file.Close()
	e.file = nil
	return err
}

type OTLPEvent struct {
	Attributes map[string]any `json:"attributes"`
}

type OTLPExporter struct{}

func NewOTLPExporter() *OTLPExporter {
	return &OTLPExporter{}
}

func (e *OTLPExporter) Export(event models.ToolEvent) error {
	_ = event
	return nil
}

func (e *OTLPExporter) Close() error {
	return nil
}

func ToOTLP(event models.ToolEvent) OTLPEvent {
	return OTLPEvent{
		Attributes: map[string]any{
			"traceai.tool.name":        event.ToolName,
			"traceai.tool.type":        event.ToolType,
			"traceai.tool.success":     event.Success,
			"traceai.tool.duration_ms": event.DurationMS,
			"traceai.agent.name":       event.AgentName,
			"traceai.error.code":       event.ErrorCode,
			"traceai.error.type":       event.ErrorType,
		},
	}
}
