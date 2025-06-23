package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, 1<<30)
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

	// _, err = processVideoForFastStart(tempFile.Name())
	// if err != nil {
	// 	log.Printf("could not process video for fast start: %v", err)
	// 	w.WriteHeader(500)
	// 	return
	// }

	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	io.Copy(tempFile, formedFile)
	tempFile.Seek(0, io.SeekStart)

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		log.Printf("could not get video aspect ratio: %v", err)
		w.WriteHeader(500)
		return
	}
	var ratioName string
	switch aspectRatio {
	case "16:9":
		ratioName = "landscape"
	case "9:16":
		ratioName = "portrait"
	case "other":
		ratioName = "other"
	}

	bucketName := os.Getenv("S3_BUCKET")
	randID := make([]byte, 32)
	rand.Read(randID)
	trueRandID := base64.RawStdEncoding.EncodeToString(randID)
	randerestID := strings.ReplaceAll(trueRandID, "/", "")
	randerestID = strings.ReplaceAll(randerestID, "+", "")
	stringFileKey := ratioName + "/" + randerestID + ".mp4"

	wuh, err := cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &bucketName,
		Key:         &stringFileKey,
		Body:        tempFile,
		ContentType: &mediaType,
	})
	if err != nil {
		log.Printf("could not put object into bucket: %v", err)
		w.WriteHeader(500)
		return
	}

	newURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s",
		os.Getenv("S3_BUCKET"),
		os.Getenv("S3_REGION"),
		stringFileKey)

	video.ID = vidUUID
	video.UpdatedAt = time.Now()
	video.VideoURL = &newURL

	fmt.Printf("wuh: %v\n", wuh)

	fmt.Printf("video details: %v\n", video)

	cfg.db.UpdateVideo(video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmdExec := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	newBuff := bytes.Buffer{}
	cmdExec.Stdout = &newBuff
	err := cmdExec.Run()
	if err != nil {
		log.Printf("could not run aspect ratio command: %v", err)
		return "", fmt.Errorf("could not run aspect ratio command: %v", err)
	}
	type aspectRatio struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	streams := struct {
		Streams []aspectRatio `json:"streams"`
	}{
		Streams: []aspectRatio{},
	}

	fmt.Print(newBuff.String())
	json.Unmarshal(newBuff.Bytes(), &streams)
	ratio := float64(streams.Streams[0].Width) / float64(streams.Streams[0].Height)
	fmt.Printf("ratio!!!!!: %v\n", ratio)
	if ratio <= 1.78 && ratio >= 1.76 {
		return "16:9", nil
	}
	if ratio <= .57 && ratio >= .55 {
		return "9:16", nil
	}
	return "other", nil
}

// func processVideoForFastStart(filePath string) (string, error) {
// 	outputFilePath := filePath + ".processing"
// 	cmd := exec.Command("ffmpeg", "-i", "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
// 	err := cmd.Run()
// 	var newPath []byte
// 	cmd.Stdout.Write(newPath)
// 	if err != nil {
// 		return "", err
// 	}
// 	if len(newPath) == 0 {
// 		return "", errors.New("no new path to return")
// 	}
// 	file, err := os.Open(string(newPath))
// 	oldFile, err := os.Open(filePath)
// 	os.RemoveAll(oldFile.Name())
// 	num, err := io.Copy(oldFile, file)
// 	if num == 0 || err != nil {
// 		log.Printf("could not copy new file to old file path")
// 		return "", err
// 	}
// 	return filePath, nil
// }
