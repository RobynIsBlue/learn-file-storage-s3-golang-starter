package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1<<30)
	// videoJSON := database.Video{}
	// json.NewDecoder(r.Body).Decode(&videoJSON)
	vidUUID, err := uuid.Parse(r.PathValue("videoID"))
	if err != nil {
		log.Printf("could not find video id: %v", err)
		w.WriteHeader(400)
		return
	}
	bearToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("could not find bearer token header: %v", err)
		w.WriteHeader(400)
		return
	}

	userID, err := auth.ValidateJWT(bearToken, cfg.jwtSecret)
	if err != nil {
		log.Printf("could not validate JWT: %v", err)
		w.WriteHeader(400)
		return
	}

	// could just throw an unneccessary error
	// if videoJSON.UserID != userID {
	// 	log.Printf("user IDs don't match: %v", err)
	// 	return
	// }

	video, err := cfg.db.GetVideo(vidUUID)
	if err != nil {
		log.Printf("could not find video with given uuid: %v", err)
		w.WriteHeader(400)
		return
	}
	fmt.Printf("video ID: %s\n", video.ID)
	if video.UserID != userID {
		log.Printf("video's user ID and given user ID do not match: %v, %v, %v", err, video.UserID, userID)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	formedFile, formedFileHeaders, err := r.FormFile("video")
	if err != nil {
		log.Printf("could not form file: %v", err)
		w.WriteHeader(500)
		return
	}
	defer formedFile.Close()
	mediaType, _, err := mime.ParseMediaType(formedFileHeaders.Header.Get("Content-Type"))
	if mediaType != "video/mp4" {
		log.Printf("media is not video: %v", err)
		w.WriteHeader(400)
		return
	}
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		log.Printf("could not create temp file: %v", err)
		w.WriteHeader(500)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	io.Copy(tempFile, formedFile)
	tempFile.Seek(0, io.SeekStart)

	bucketName := os.Getenv("S3_BUCKET")
	fileKey := []byte{}
	rand.Read(fileKey)
	stringFileKey := string(fileKey)
	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &bucketName,
		Key:         &stringFileKey,
		Body:        tempFile,
		ContentType: &mediaType,
	})

	newURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
		os.Getenv("S3_BUCKET"),
		os.Getenv("S3_REGION"),
		stringFileKey)
	newVid := database.Video{
		ID:        vidUUID,
		UpdatedAt: time.Now(),
		VideoURL:  &newURL,
	}
	cfg.db.UpdateVideo(newVid)
}
