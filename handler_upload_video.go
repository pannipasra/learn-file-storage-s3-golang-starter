package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// Set an upload limit of 1 GB
	const uploadLimit = 1 << 30
	http.MaxBytesReader(w, r.Body, uploadLimit)

	// Extract videoID
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// Authenticate the user to get a userID
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	// Get video's metadata from SQLite database
	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video's metadata", err)
		return
	}

	// Validate is current user is onwer of the video
	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to access this video", nil)
		return
	}

	// Parse the uploaded video file from the form data
	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to read file: %v", err), nil)
		return
	}
	defer file.Close()

	// Get Content-Type
	contentType := header.Header.Get("Content-Type")

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", err)
		return
	}

	// Validate ensure it's an MP4 video
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Only MP4 is supported", nil)
		return
	}

	// Save the uploaded file to a temporary file on disk.
	fileTemp, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create temporary file on disk", err)
		return
	}
	defer os.Remove(fileTemp.Name()) // Ensure the temporary file is removed
	defer fileTemp.Close()           // Ensure the temporary file is closed

	_, err = io.Copy(fileTemp, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process created temporary file on disk", err)
		return
	}

	_, err = fileTemp.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to process created temporary file on disk", err)
		return
	}

	//  Put object into s3
	randomBytes := make([]byte, 32)
	objectKey := fmt.Sprintf("%v.%v", hex.EncodeToString(randomBytes), mediaType)
	putObjectInput := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &objectKey,
		Body:        fileTemp,
		ContentType: &mediaType,
	}
	cfg.s3Client.PutObject(r.Context(), putObjectInput)

	// Update the VideoURL of the video record in the database with the S3 bucket
	videoURL := fmt.Sprintf("https://%v.s3.%v.amazonaws.com/%v", cfg.s3Bucket, cfg.s3Region, objectKey)
	videoMetadata.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can not update video metadata", err)
		return
	}

	// Respond with updated JSON of the video's metadata
	respondWithJSON(w, http.StatusOK, videoMetadata)
}
