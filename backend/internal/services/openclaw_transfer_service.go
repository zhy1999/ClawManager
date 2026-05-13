package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"clawreef/internal/services/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const openclawConfigDirName = ".openclaw"

type OpenClawTransferService interface {
	Export(ctx context.Context, userID, instanceID int) ([]byte, error)
	Import(ctx context.Context, userID, instanceID int, archive io.Reader) error
}

type openClawTransferService struct {
	podService *k8s.PodService
}

func NewOpenClawTransferService() OpenClawTransferService {
	return &openClawTransferService{
		podService: k8s.NewPodService(),
	}
}

func (s *openClawTransferService) Export(ctx context.Context, userID, instanceID int) ([]byte, error) {
	command := []string{
		"sh",
		"-lc",
		fmt.Sprintf("target_dir=${HOME:-/home/user}/%[1]s; test -d \"$target_dir\" && tar czf - -C \"${HOME:-/home/user}\" %[1]s", shellQuote(openclawConfigDirName)),
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := s.exec(ctx, userID, instanceID, command, nil, &stdout, &stderr); err != nil {
		return nil, formatExecError("export .openclaw", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

func (s *openClawTransferService) Import(ctx context.Context, userID, instanceID int, archive io.Reader) error {
	command := []string{
		"sh",
		"-lc",
		fmt.Sprintf("home_dir=${HOME:-/home/user}; target_dir=\"$home_dir/%[1]s\"; rm -rf \"$target_dir\" && mkdir -p \"$home_dir\" && tar xzf - -C \"$home_dir\"", shellQuote(openclawConfigDirName)),
	}

	var stderr bytes.Buffer
	if err := s.exec(ctx, userID, instanceID, command, archive, nil, &stderr); err != nil {
		return formatExecError("import .openclaw", err, stderr.String())
	}

	return nil
}

func (s *openClawTransferService) exec(ctx context.Context, userID, instanceID int, command []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if s.podService == nil || s.podService.GetClient() == nil || s.podService.GetClient().Clientset == nil {
		return fmt.Errorf("k8s client not initialized")
	}

	pod, err := s.podService.GetPod(ctx, userID, instanceID)
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	req := s.podService.GetClient().Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{
		Container: "desktop",
		Command:   command,
		Stdin:     stdin != nil,
		Stdout:    stdout != nil,
		Stderr:    stderr != nil,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(s.podService.GetClient().Config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to initialize exec stream: %w", err)
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func formatExecError(action string, execErr error, stderr string) error {
	if stderr != "" {
		return fmt.Errorf("failed to %s: %s", action, stderr)
	}
	return fmt.Errorf("failed to %s: %w", action, execErr)
}
