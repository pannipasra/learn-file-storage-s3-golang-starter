package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here

	// Parse the form data: validate
	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	// Get the image data from the form
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Get Content-Type
	contentType := header.Header.Get("Content-Type")

	// Get the video's metadata from the SQLite database
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

	// Upload file to /assets/<videoID>.<file_extension>
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid content type", err)
		return
	}

	// Only allow image/jpeg and image/png
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Only JPEG and PNG images are supported", nil)
		return
	}

	// Extract file extension from media type (e.g., "image/png" -> "png")
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type format", nil)
		return
	}
	fileExtension := parts[1]

	// Make unique file name
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomName := base64.RawURLEncoding.EncodeToString(randomBytes)

	fileName := fmt.Sprintf("%s.%s", randomName, fileExtension)
	pathToUpload := filepath.Join(cfg.assetsRoot, fileName)

	// Create a new file
	newFile, err := os.Create(pathToUpload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can not create a new file", err)
		return
	}
	defer newFile.Close() // Important: close the file when done

	// Copy the contents from the multipart.File to the new file on disk
	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Cannot write file contents", err)
		return
	}

	// Update the video metadata
	thumbnailURL := fmt.Sprintf("http://localhost:%v/assets/%v", 8091, fileName)
	videoMetadata.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can not update video metadata", err)
		return
	}

	// Respond with updated JSON of the video's metadata
	respondWithJSON(w, http.StatusOK, videoMetadata)
}
