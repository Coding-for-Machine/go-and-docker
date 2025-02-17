package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
)

type Submission struct {
	UserCode  string `json:"user_code"`
	Language  string `json:"language"`
	TestCases string `json:"test_cases"`
}

var languageCommands = map[string]string{
	"python": "python3 /app/solution.py",
	"go":     "go run /app/solution.go",
}

func getFileName(language string) string {
	if language == "python" {
		return "solution.py"
	} else if language == "go" {
		return "solution.go"
	}
	return "solution.txt"
}

func createTarFile(fileName, content string) (*bytes.Buffer, error) {
	buffer := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buffer)
	defer tarWriter.Close()

	hdr := &tar.Header{
		Name: fileName,
		Mode: 0600,
		Size: int64(len(content)),
	}
	if err := tarWriter.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("Tar header yozishda xatolik: %v", err)
	}
	if _, err := tarWriter.Write([]byte(content)); err != nil {
		return nil, fmt.Errorf("Tar fayl yozishda xatolik: %v", err)
	}

	return buffer, nil
}

func fileConnect(cli *client.Client, containerName, fileName, containerPath, content string) error {
	tarBuffer, err := createTarFile(fileName, content)
	if err != nil {
		return fmt.Errorf("Tar fayl yaratishda xatolik: %v", err)
	}

	return cli.CopyToContainer(context.Background(), containerName, containerPath, tarBuffer, types.CopyToContainerOptions{})
}

func getMemoryUsage() (int, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, fmt.Errorf("Memory usage o'qib bo'lmadi: %v", err)
	}

	re := regexp.MustCompile(`VmRSS:\s+(\d+) kB`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) < 2 {
		return 0, fmt.Errorf("Memory usage topilmadi")
	}
	return strconv.Atoi(matches[1])
}

func executeCode(cli *client.Client, containerName, command string) (map[string]interface{}, error) {
	startTime := time.Now()
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          []string{"sh", "-c", command},
	}
	execIDResp, err := cli.ContainerExecCreate(context.Background(), containerName, execConfig)
	if err != nil {
		return nil, fmt.Errorf("Exec yaratishda xatolik: %v", err)
	}

	resp, err := cli.ContainerExecAttach(context.Background(), execIDResp.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, fmt.Errorf("Exec attach qilishda xatolik: %v", err)
	}
	defer resp.Close()

	var outputBuffer bytes.Buffer
	if _, err := io.Copy(&outputBuffer, resp.Reader); err != nil {
		return nil, fmt.Errorf("Natijani o'qishda xatolik: %v", err)
	}

	executionTime := time.Since(startTime).Seconds()
	memoryUsage, _ := getMemoryUsage()

	output := strings.TrimSpace(outputBuffer.String())
	isAccepted := strings.Contains(output, "OK")

	result := map[string]interface{}{
		"memory":     memoryUsage,
		"is_accepted": isAccepted,
		"time":       executionTime,
		"details":    output,
	}

	return result, nil
}

func main() {
	app := fiber.New()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Docker client yaratishda xatolik: %v", err)
	}
	defer cli.Close()

	app.Post("/run-test", func(c *fiber.Ctx) error {
		var submission Submission
		if err := c.BodyParser(&submission); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON format"})
		}

		command, exists := languageCommands[submission.Language]
		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unsupported language"})
		}

		containerName := fmt.Sprintf("%s-app", submission.Language)
		fileName := getFileName(submission.Language)

		fullCode := fmt.Sprintf("%s\n%s", submission.UserCode, submission.TestCases)
		if err := fileConnect(cli, containerName, fileName, "/app/", fullCode); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		result, err := executeCode(cli, containerName, command)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(result)
	})

	log.Fatal(app.Listen(":3000"))
}

