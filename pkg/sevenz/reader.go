package sevenz

import (
	"io"
	"os/exec"
)

// cmdReadCloser оборачивает stdout процесса и корректно завершает его при Close.
type cmdReadCloser struct {
	pipe io.ReadCloser
	cmd  *exec.Cmd
}

func (c *cmdReadCloser) Read(p []byte) (int, error) {
	return c.pipe.Read(p)
}

func (c *cmdReadCloser) Close() error {
	// Закрываем pipe, чтобы 7z получил EOF на stdout и мог корректно завершиться
	pipeErr := c.pipe.Close()
	// Wait() дожидается завершения процесса (или возвращает ошибку контекста, если он был отменён)
	waitErr := c.cmd.Wait()
	if pipeErr != nil {
		return pipeErr
	}
	return waitErr
}
