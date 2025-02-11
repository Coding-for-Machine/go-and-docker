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

// 📌 TestCase tuzilmasi
type TestCase struct {
	TestCase       int    `json:"test_case"`
	Input          []int  `json:"input"`
	ExpectedOutput int    `json:"expected_output"`
	ActualOutput   int    `json:"actual_output,omitempty"`
	Passed         bool   `json:"passed,omitempty"`
	Language       string `json:"language"`
}

// 🔹 Qo'llab-quvvatlanadigan tillar va ularning run buyruqlari
var languageCommands = map[string]string{
	"python": "python3 /app/solution.py",
	"go":     "go run /app/solution.go",
}

// 📌 Tar formatida fayl yaratish
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

// 📌 Faylni konteynerga joylash
func fileConnect(cli *client.Client, containerName, filePath, containerPath, fileContent string) error {
	tarBuffer, err := createTarFile(filePath, fileContent)
	if err != nil {
		return fmt.Errorf("Faylni tar formatiga o'zgartirishda xatolik: %v", err)
	}

	if err := cli.CopyToContainer(context.Background(), containerName, containerPath, tarBuffer, types.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("Faylni konteynerga uzatishda xatolik: %v", err)
	}

	fmt.Printf("Fayl '%s' konteynerning '%s' yo'liga uzatildi.\n", filePath, containerPath)
	return nil
}

// 📌 Docker konteynerida kodni ishga tushirish
func codeRunIn(cli *client.Client, containerName, command, input string) (string, string, error) {
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Cmd:          []string{"sh", "-c", command},
		Tty:          false,
	}

	execIDResp, err := cli.ContainerExecCreate(context.Background(), containerName, execConfig)
	if err != nil {
		return "", "", fmt.Errorf("Exec yaratishda xatolik: %v", err)
	}

	resp, err := cli.ContainerExecAttach(context.Background(), execIDResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", "", fmt.Errorf("Exec boshlashda xatolik: %v", err)
	}
	defer resp.Close()

	if _, err := resp.Conn.Write([]byte(input + "\n")); err != nil {
		return "", "", fmt.Errorf("Input uzatishda xatolik: %v", err)
	}

	var output, errorOutput strings.Builder
	io.Copy(&output, resp.Reader)

	return strings.TrimSpace(output.String()), strings.TrimSpace(errorOutput.String()), nil
}

func main() {
	app := fiber.New()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Docker client yaratishda xatolik: %v", err)
	}
	defer cli.Close()

	containerName := "code-runner"

	// 📌 Testlarni bajarish API
	app.Post("/run-test", func(c *fiber.Ctx) error {
		var testCase TestCase
		if err := c.BodyParser(&testCase); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid JSON format"})
		}

		// 🔹 Tildan kelib chiqib fayl yaratish
		fileContent := ""
		filePath := ""
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
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Supported languages: python, go"})
		}

		// 📌 Faylni konteynerga yuklash
		if err := fileConnect(cli, containerName, filePath, "/app/", fileContent); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// 📌 Dockerda kodni ishga tushirish
		command := languageCommands[testCase.Language]
		input := fmt.Sprintf("%d,%d", testCase.Input[0], testCase.Input[1])
		output, errorOutput, err := codeRunIn(cli, containerName, command, input)
		if err != nil {
			testCase.Passed = false
			testCase.ActualOutput = 0
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		fmt.Println(errorOutput)

		// 📌 Natijani formatlash
		var actualOutput int
		fmt.Sscanf(output, "%d", &actualOutput)
		testCase.ActualOutput = actualOutput
		testCase.Passed = (actualOutput == testCase.ExpectedOutput)

		// 📌 Chiroyli JSON formatda qaytarish
		return c.JSON(testCase)
	})

	// 🌍 Serverni ishga tushirish
	port := 3000
	fmt.Printf("🚀 Server http://localhost:%d da ishlayapti...\n", port)
	log.Fatal(app.Listen(fmt.Sprintf(":%d", port)))
}
