package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxFileSize  = 10 << 20 // 10 MB
	uploadDir    = "./uploads"
	allowedTypes = "image/jpeg,image/png,application/pdf"
)

type PredictionResponse struct {
	XMM               float64  `json:"x_mm"`
	YMM               float64  `json:"y_mm"`
	AngleDeg          float64  `json:"angle_deg"`
	SegmentationMasks []string `json:"segmentation_masks"`
}

func main() {
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create upload directory: %v", err)
	}

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	router.GET("/multiple", func(c *gin.Context) {
		c.HTML(http.StatusOK, "multiple.html", nil)
	})

	router.POST("/upload", handleSingleUpload)

	router.POST("/upload-multiple", handleMultipleUpload)

	router.Static("/files", uploadDir)

	fmt.Println("Server running on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func handleSingleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"message": "No file uploaded"})
		return
	}

	if valid, err := validateFile(file); !valid {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"message": err.Error()})
		return
	}

	dst := filepath.Join(uploadDir, file.Filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "Could not save file"})
		return
	}

	// === Отправляем файл на Flask API ===
	flaskURL := "http://model-ai:5000/predict"

	resp, err := postFileToFlask(flaskURL, "file", dst)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "Failed to call prediction API"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"message": fmt.Sprintf("Prediction API error: %s", string(bodyBytes)),
		})
		return
	}

	var prediction PredictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&prediction); err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"message": "Failed to parse prediction response"})
		return
	}

	// === Рендерим страницу с результатами и ссылками на маски ===
	c.HTML(http.StatusOK, "success.html", gin.H{
		"message":            "File uploaded and predicted successfully!",
		"filename":           file.Filename,
		"size":               formatFileSize(file.Size),
		"x_mm":               fmt.Sprintf("%.2f", prediction.XMM),
		"y_mm":               fmt.Sprintf("%.2f", prediction.YMM),
		"angle_deg":          fmt.Sprintf("%.2f", prediction.AngleDeg),
		"segmentation_masks": prediction.SegmentationMasks,
	})
}

func handleMultipleUpload(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"message": "Invalid form data",
		})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{
			"message": "No files uploaded",
		})
		return
	}

	successFiles := []gin.H{}
	errorFiles := []string{}

	for _, file := range files {

		if valid, err := validateFile(file); !valid {
			errorFiles = append(errorFiles, fmt.Sprintf("%s: %s", file.Filename, err.Error()))
			continue
		}

		dst := filepath.Join(uploadDir, file.Filename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			errorFiles = append(errorFiles, fmt.Sprintf("%s: %s", file.Filename, "save failed"))
			continue
		}

		successFiles = append(successFiles, gin.H{
			"filename": file.Filename,
			"size":     formatFileSize(file.Size),
			"url":      "/files/" + file.Filename,
		})
	}

	c.HTML(http.StatusOK, "multiple-result.html", gin.H{
		"successCount": len(successFiles),
		"errorCount":   len(errorFiles),
		"successFiles": successFiles,
		"errorFiles":   errorFiles,
	})
}

// helper функция для POST с файлом
func postFileToFlask(url, fieldname, filepath string) (*http.Response, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldname, path.Base(filepath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	return client.Do(req)
}

func validateFile(file *multipart.FileHeader) (bool, error) {

	if file.Size > maxFileSize {
		return false, fmt.Errorf("file too large (max %dMB)", maxFileSize/(1<<20))
	}

	contentType := file.Header.Get("Content-Type")
	if !isAllowedType(contentType) {
		return false, fmt.Errorf("invalid file type: %s", contentType)
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !isAllowedExtension(ext) {
		return false, fmt.Errorf("invalid file extension: %s", ext)
	}

	return true, nil
}

func isAllowedType(contentType string) bool {
	allowed := strings.Split(allowedTypes, ",")
	for _, t := range allowed {
		if contentType == t {
			return true
		}
	}
	return false
}

func isAllowedExtension(ext string) bool {
	allowed := []string{".jpg", ".jpeg", ".png", ".pdf"}
	for _, e := range allowed {
		if ext == e {
			return true
		}
	}
	return false
}

func formatFileSize(size int64) string {
	const unit = 1000
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "kMGTPE"[exp])
}
