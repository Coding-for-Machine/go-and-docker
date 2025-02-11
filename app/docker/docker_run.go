package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
)

func DockerRun(c *fiber.Ctx) error {
	return c.SendString("Post")
}

// TestCase tuzilmasi
type TestCase struct {
	TestCase       int   `json:"test_case"`
	Input          []int `json:"input"`
	ExpectedOutput int   `json:"expected_output"`
	ActualOutput   int   `json:"actual_output"`
	Passed         bool  `json:"passed"`
}

// Docker konteynerini tekshirish
func getExistingContainer(cli *client.Client, containerName string) (*types.ContainerJSON, error) {
	container, err := cli.ContainerInspect(context.Background(), containerName)
	if err != nil {
		return nil, fmt.Errorf("Container %s topilmadi: %v", containerName, err)
	}
	fmt.Printf("[INFO] Container %s topildi.\n", containerName)
	return &container, nil
}

// Tar formatida fayl yaratish
func createTarFile(fileName, content string) (*bytes.Buffer, error) {
	buffer := new(bytes.Buffer)
	tarWriter := tar.NewWriter(buffer)

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

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("Tar yozishni yopishda xatolik: %v", err)
	}

	return buffer, nil
}

// Faylni konteynerga yuborish
func fileConnect(cli *client.Client, containerName, filePath, containerPath string) error {

	fileContent := `import sys
input_data = sys.stdin.read().strip().split(',')
num1, num2 = map(int, input_data)
print(num1 + num2)`

	tarBuffer, err := createTarFile(filePath, fileContent)
	if err != nil {
		return fmt.Errorf("Faylni tar formatiga o'zgartirishda xatolik: %v", err)
	}

	err = cli.CopyToContainer(context.Background(), containerName, containerPath, tarBuffer, types.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("Faylni konteynerga uzatishda xatolik: %v", err)
	}

	fmt.Printf("[INFO] Fayl '%s' konteynerning '%s' yo'liga uzatildi.\n", filePath, containerPath)
	return nil
}

// Kodni konteyner ichida ishga tushirish
func codeRunIn(cli *client.Client, containerName, fileName string, input string) (string, string, error) {
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Cmd:          []string{"python", fmt.Sprintf("/app/%s", fileName)},
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
	done := make(chan struct{})

	go func() {
		_, _ = io.Copy(&output, resp.Reader)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second): // Maksimal 5 soniya kutish
		return "", "", fmt.Errorf("Exec timeout")
	}

	return strings.TrimSpace(output.String()), strings.TrimSpace(errorOutput.String()), nil
}

// Asosiy ishga tushirish
func main() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		fmt.Printf("[ERROR] Docker client yaratishda xatolik: %v\n", err)
		return
	}
	defer cli.Close()

	containerName := "python-app"
	_, err = getExistingContainer(cli, containerName)
	if err != nil {
		fmt.Println(err)
		return
	}

	containerPath := "/app/"
	filePath := "solution.py"
	if err := fileConnect(cli, containerName, filePath, containerPath); err != nil {
		fmt.Println(err)
		return
	}

	testCases := []TestCase{
		{TestCase: 1, Input: []int{3, 5}, ExpectedOutput: 8},
		{TestCase: 2, Input: []int{-1, 1}, ExpectedOutput: 0},
		{TestCase: 3, Input: []int{0, 0}, ExpectedOutput: 0},
		{TestCase: 4, Input: []int{100, 200}, ExpectedOutput: 300},
	}

	for i, testCase := range testCases {
		input := fmt.Sprintf("%d,%d", testCase.Input[0], testCase.Input[1])
		output, errorOutput, err := codeRunIn(cli, containerName, filePath, input)
		if err != nil {
			testCases[i].Passed = false
			testCases[i].ActualOutput = 0
			fmt.Printf("[ERROR] Test case %d xatolik bilan yakunlandi: %v\n", testCase.TestCase, err)
			continue
		}

		var actualOutput int
		fmt.Sscanf(output, "%d", &actualOutput)
		testCases[i].ActualOutput = actualOutput
		testCases[i].Passed = (actualOutput == testCase.ExpectedOutput)

		if len(errorOutput) > 0 {
			fmt.Printf("[WARNING] Test case %d stderr chiqishi:\n%s\n", testCase.TestCase, errorOutput)
		}

		status := "✅ PASSED"
		if !testCases[i].Passed {
			status = "❌ FAILED"
		}
		fmt.Printf("[INFO] Test case %d: %s (Expected: %d, Got: %d)\n", testCase.TestCase, status, testCase.ExpectedOutput, actualOutput)
	}

	jsonResult, _ := json.MarshalIndent(testCases, "", "  ")
	fmt.Printf("\nNatija:\n%s\n", string(jsonResult))
}
