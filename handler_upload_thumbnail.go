package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

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
	pathToUpload := filepath.Join(cfg.assetsRoot, videoIDString, contentType)

	// Create a new file
	newFile, err := os.Create(pathToUpload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can not create a new file", err)
		return
	}

	// Copy the contents from the multipart.File to the new file on disk
	io.Copy(newFile, file)

	// Update the video metadata
	thumbnailURL := fmt.Sprintf("http://localhost:%v/assets/%v.%v", 8091, videoIDString, contentType)
	videoMetadata.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can not update video metadata", err)
		return
	}

	// Respond with updated JSON of the video's metadata
	respondWithJSON(w, http.StatusOK, videoMetadata)
}
