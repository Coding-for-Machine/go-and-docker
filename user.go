package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
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

func parseTestOutput(output string) map[string]interface{} {
	result := map[string]interface{}{
		"tests_run":    0,
		"tests_passed": 0,
		"tests_failed": 0,
		"details":      output,
	}

	testsRunRe := regexp.MustCompile(`Ran (\d+) test[s]?`)
	passRe := regexp.MustCompile(`OK`)
	failRe := regexp.MustCompile(`FAILED`)

	if match := testsRunRe.FindStringSubmatch(output); len(match) > 1 {
		result["tests_run"], _ = strconv.Atoi(match[1])
	}
	if passRe.MatchString(output) {
		result["tests_passed"] = result["tests_run"].(int)
	}
	if failRe.MatchString(output) {
		result["tests_failed"] = result["tests_run"].(int) - result["tests_passed"].(int)
	}

	return result
}

func executeCode(cli *client.Client, containerName, command string) (map[string]interface{}, float64, int, error) {
	startTime := time.Now()
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
		Cmd:          []string{"sh", "-c", command},
	}
	execIDResp, err := cli.ContainerExecCreate(context.Background(), containerName, execConfig)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Exec yaratishda xatolik: %v", err)
	}

	resp, err := cli.ContainerExecAttach(context.Background(), execIDResp.ID, types.ExecStartCheck{})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Exec attach qilishda xatolik: %v", err)
	}
	defer resp.Close()

	var outputBuffer bytes.Buffer
	if _, err := io.Copy(&outputBuffer, resp.Reader); err != nil {
		return nil, 0, 0, fmt.Errorf("Natijani o'qishda xatolik: %v", err)
	}

	executionTime := time.Since(startTime).Seconds()
	memoryUsage, _ := getMemoryUsage()

	parsedOutput := parseTestOutput(outputBuffer.String())

	return parsedOutput, executionTime, memoryUsage, nil
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

		output, execTime, memoryUsage, err := executeCode(cli, containerName, command)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"output":   output,
			"time":     execTime,
			"memory":   memoryUsage,
		})
	})

	log.Fatal(app.Listen(":3000"))
}

