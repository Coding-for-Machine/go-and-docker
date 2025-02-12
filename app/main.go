package main

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
)

type TestCase struct {
	TestCase       int    `json:"test_case"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	ActualOutput   string `json:"actual_output,omitempty"`
	Passed         bool   `json:"passed,omitempty"`
	Language       string `json:"language"`
}

var languageCommands = map[string]string{
	"python": "python3 /app/solution.py",
	"go":     "go run /app/solution.go",
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

func fileConnect(cli *client.Client, containerName, filePath, containerPath, fileContent string) error {
	tarBuffer, err := createTarFile(filePath, fileContent)
	if err != nil {
		return fmt.Errorf("Faylni tar formatiga o'zgartirishda xatolik: %v", err)
	}

	if err := cli.CopyToContainer(context.Background(), containerName, containerPath, tarBuffer, types.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("Faylni konteynerga uzatishda xatolik: %v", err)
	}
	return nil
}

func executeCode(cli *client.Client, containerName, command, input string) (string, error) {
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Tty:          false,
		Cmd:          []string{"sh", "-c", fmt.Sprintf("echo -n '%s' | %s", input, command)},
	}

	execIDResp, err := cli.ContainerExecCreate(context.Background(), containerName, execConfig)
	if err != nil {
		return "", fmt.Errorf("Exec yaratishda xatolik: %v", err)
	}

	resp, err := cli.ContainerExecAttach(context.Background(), execIDResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("Exec attach qilishda xatolik: %v", err)
	}
	defer resp.Close()

	var outputBuffer bytes.Buffer
	if _, err := io.Copy(&outputBuffer, resp.Reader); err != nil {
		return "", fmt.Errorf("Natijani o'qishda xatolik: %v", err)
	}

	return strings.TrimSpace(outputBuffer.String()), nil
}

func main() {
	app := fiber.New()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Docker client yaratishda xatolik: %v", err)
	}
	defer cli.Close()

	app.Post("/run-test", func(c *fiber.Ctx) error {
		var testCase TestCase
		if err := c.BodyParser(&testCase); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON format"})
		}

		var fileContent, filePath string
		if testCase.Language == "python" {
			filePath = "solution.py"
			fileContent = `import sys
input_data = sys.stdin.read().strip().split(',')
num1, num2 = map(int, input_data)
print(num1 + num2)`
		} else if testCase.Language == "go" {
			filePath = "solution.go"
			fileContent = `package main
import ("fmt"; "os"; "strings"; "strconv")
func main() {
	data, _ := os.Stdin.ReadString('\n')
	nums := strings.Split(strings.TrimSpace(data), ",")
	a, _ := strconv.Atoi(nums[0])
	b, _ := strconv.Atoi(nums[1])
	fmt.Println(a + b)
}`
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unsupported language"})
		}

		containerName := fmt.Sprintf("%s-app", testCase.Language)
		command, exists := languageCommands[testCase.Language]
		if !exists {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Unsupported language"})
		}

		if err := fileConnect(cli, containerName, filePath, "/app/", fileContent); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		actualOutput, err := executeCode(cli, containerName, command, testCase.Input)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		testCase.ActualOutput = actualOutput
		testCase.Passed = (testCase.ActualOutput == testCase.ExpectedOutput)

		return c.JSON(testCase)
	})

	log.Fatal(app.Listen(":3000"))
}
