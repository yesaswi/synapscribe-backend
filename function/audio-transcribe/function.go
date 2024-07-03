package audiotranscription

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// StorageObjectData contains metadata of the Cloud Storage object.
type StorageObjectData struct {
	Bucket         string    `json:"bucket,omitempty"`
	Name           string    `json:"name,omitempty"`
	Metageneration int64     `json:"metageneration,string,omitempty"`
	TimeCreated    time.Time `json:"timeCreated,omitempty"`
	Updated        time.Time `json:"updated,omitempty"`
}

func init() {
	functions.CloudEvent("AudioTranscription", AudioTranscription)
}

func logJSON(message string, severity string) {
	logEntry := struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
	}{
		Message:  message,
		Severity: severity,
	}
	jsonLog, _ := json.Marshal(logEntry)
	fmt.Println(string(jsonLog))
}

func AudioTranscription(ctx context.Context, e event.Event) error {
	logJSON(fmt.Sprintf("Event ID: %s", e.ID()), "INFO")
	logJSON(fmt.Sprintf("Event Type: %s", e.Type()), "INFO")

	var data StorageObjectData
	if err := e.DataAs(&data); err != nil {
		logJSON(fmt.Sprintf("event.DataAs: %v", err), "ERROR")
		return fmt.Errorf("event.DataAs: %v", err)
	}

	logJSON(fmt.Sprintf("Bucket: %s", data.Bucket), "INFO")
	logJSON(fmt.Sprintf("File: %s", data.Name), "INFO")
	logJSON(fmt.Sprintf("Metageneration: %d", data.Metageneration), "INFO")
	logJSON(fmt.Sprintf("Created: %s", data.TimeCreated), "INFO")
	logJSON(fmt.Sprintf("Updated: %s", data.Updated), "INFO")

	gcsURL := fmt.Sprintf("gs://%s/%s", data.Bucket, data.Name)
	
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("GEMINI_API_KEY")))
	if err != nil {
		logJSON(fmt.Sprintf("Failed to create Gemini client: %v", err), "ERROR")
		return fmt.Errorf("failed to create Gemini client: %v", err)
	}
	defer client.Close()

	transcription, err := transcribeAudio(ctx, client, gcsURL)
	if err != nil {
		logJSON(fmt.Sprintf("Failed to transcribe audio: %v", err), "ERROR")
		return fmt.Errorf("failed to transcribe audio: %v", err)
	}

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		logJSON(fmt.Sprintf("Failed to create GCS client: %v", err), "ERROR")
		return fmt.Errorf("failed to create GCS client: %v", err)	
	}
	defer gcsClient.Close()

	// Save the response to a file in a Cloud Storage bucket
	bucket := os.Getenv("TRANSCRIPTION_BUCKET")
	object := fmt.Sprintf("transcription-%s.txt", data.Name)
	obj := gcsClient.Bucket(bucket).Object(object)
	writer := obj.NewWriter(ctx)
	defer writer.Close()
	if _, err := writer.Write([]byte(transcription)); err != nil {
		logJSON(fmt.Sprintf("Failed to write transcription to GCS: %v", err), "ERROR")
		return fmt.Errorf("failed to write transcription to GCS: %v", err)
	}

	logJSON("Transcription completed successfully", "INFO")
	return nil
}

func transcribeAudio(ctx context.Context, client *genai.Client, gcsURL string) (string, error) {
	model := client.GenerativeModel("gemini-1.5-pro")
	model.SetTemperature(0.4)
	model.SafetySettings = []*genai.SafetySetting{
		{
			Category:  genai.HarmCategoryHarassment,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryHateSpeech,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategorySexuallyExplicit,
			Threshold: genai.HarmBlockNone,
		},
		{
			Category:  genai.HarmCategoryDangerousContent,
			Threshold: genai.HarmBlockNone,
		},
	}
	prompt := "Transcribe this audio file. Provide only the transcribed text without any additional formatting or speaker identification."
	
	// Parse the GCS URL to get bucket and object names
	bucket, object, err := parseGCSURL(gcsURL)
	if err != nil {
		logJSON(fmt.Sprintf("Invalid GCS URL: %v", err), "ERROR")
		return "", fmt.Errorf("invalid GCS URL: %w", err)
	}

	// Create a new GCS client
	gcsClient, err := storage.NewClient(ctx, option.WithScopes(storage.ScopeReadOnly))
	if err != nil {
		logJSON(fmt.Sprintf("Failed to create GCS client: %v", err), "ERROR")
		return "", fmt.Errorf("failed to create GCS client: %w", err)
	}
	defer gcsClient.Close()

	// Get a handle to the GCS object
	obj := gcsClient.Bucket(bucket).Object(object)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		logJSON(fmt.Sprintf("Failed to create reader for GCS object: %v", err), "ERROR")
		return "", fmt.Errorf("failed to create reader for GCS object: %w", err)
	}
	defer reader.Close()

	// Upload the file to the Gemini service
	file, err := client.UploadFile(ctx, "", reader, nil)
	if err != nil {
		logJSON(fmt.Sprintf("Unable to upload file: %v", err), "ERROR")
		return "", fmt.Errorf("unable to upload file: %w", err)
	}

	res, err := model.GenerateContent(ctx, genai.FileData{URI: file.URI}, genai.Text(prompt))
	if err != nil {
		logJSON(fmt.Sprintf("Unable to generate contents: %v", err), "ERROR")
		return "", fmt.Errorf("unable to generate contents: %w", err)
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		logJSON("Empty response from model", "ERROR")
		return "", fmt.Errorf("empty response from model")
	}

	logJSON("Audio transcription completed", "INFO")
	return fmt.Sprintf("%v", res.Candidates[0].Content.Parts[0]), nil
}

func parseGCSURL(url string) (bucket, object string, err error) {
	const prefix = "gs://"
	if !strings.HasPrefix(url, prefix) {
		return "", "", fmt.Errorf("invalid GCS URL format")
	}
	path := strings.TrimPrefix(url, prefix)
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid GCS URL format")
	}
	return parts[0], parts[1], nil
}
