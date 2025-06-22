package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

const maxMemory = 10 << 20

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

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, 400, "Couldn't parse multipart form", err)
		return
	}

	file, fileHeaders, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, 400, "Couldn't find thumbnail file or file headers", err)
		return
	}
	defer file.Close()

	contentType := fileHeaders.Header.Get("Content-Type")
	if contentType == "" {
		respondWithError(w, 400, "Couldn't find Content-Type header", errors.New("no content-type"))
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, 400, "Could not find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You do not own this video", err)
	}

	//convert file to bytes
	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, file); err != nil {
		respondWithError(w, 400, "Could not buffer thumbnail file", err)
		return
	}

	//probably going to print contentType wrong
	extension, _ := mime.ExtensionsByType(contentType)
	if extension[0] != ".jpeg" && extension[0] != ".png" {
		respondWithError(w, 400, "extension must be jpeg or png", nil)
		return
	}
	fmt.Printf("extension: %v\n", extension[0])

	randID := make([]byte, 32)
	rand.Read(randID)
	trueRandID := base64.RawStdEncoding.EncodeToString(randID)
	link := fmt.Sprintf("%s%s", trueRandID, extension[0])
	joinedPath := filepath.Join(cfg.assetsRoot, link)

	destFile, err := os.Create(joinedPath)
	if err != nil {
		respondWithError(w, 400, "Could not create file with thumbnail information", err)
		return
	}
	defer destFile.Close()

	file.Seek(0, io.SeekStart)
	_, err = io.Copy(destFile, file)
	if err != nil {
		respondWithError(w, 400, "Could not copy files to assets", err)
		return
	}

	dataURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, link)
	fmt.Println(dataURL)

	video.ThumbnailURL = &dataURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, 400, "could not update video", err)
		return
	}

	fmt.Print(video.ThumbnailURL)
	respondWithJSON(w, http.StatusOK, video.ThumbnailURL)
}
