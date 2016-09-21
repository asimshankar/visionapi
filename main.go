package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/vision/v1"
)

const (
	microsoftApiKeyEnvVar = "MICROSOFT_API_KEY"
)

func main() {
	flag.Usage = usage
	verbose := flag.Bool("v", false, "Verbose output")
	provider := flag.String("api", "auto", "Which API to use: google, microsoft or auto-detect (and possibly both)")
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		return
	}
	*provider = strings.ToLower(*provider)
	switch *provider {
	case "google":
		mainGoogle(*verbose)
	case "microsoft":
		mainMicrosoft(*verbose)
	case "auto":
		if len(os.Getenv(microsoftApiKeyEnvVar)) > 0 {
			mainMicrosoft(*verbose)
		} else {
			mainGoogle(*verbose)
		}
	default:
		log.Fatalf("Invalid --provider(%s), must be 'auto', 'google' or 'microsoft'", *provider)
	}
}

func mainMicrosoft(verbose bool) {
	client := http.DefaultClient
	key := os.Getenv(microsoftApiKeyEnvVar)
	if len(key) == 0 {
		log.Fatal("Must set %s environment variable to a valid obtained from https://www.microsoft.com/cognitive-services/en-US/subscriptions", microsoftApiKeyEnvVar)
	}
	for _, pattern := range flag.Args() {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid file pattern %s: %v", pattern, err)
			continue
		}
		for _, filename := range matches {
			byts, err := loadFile(filename)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to load %s: %v\n", filename, err)
				continue
			}
			// From:
			// https://www.microsoft.com/cognitive-services/en-us/computer-vision-api/documentation/howtocallvisionapi
			// and
			// https://dev.projectoxford.ai/docs/services/56f91f2d778daf23d8ec6739/operations/56f91f2e778daf14a499e1fa
			req, err := http.NewRequest("POST", "https://api.projectoxford.ai/vision/v1.0/analyze?visualFeatures=Description,Tags", bytes.NewReader(byts))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to create request for %s: %v\n", filename, err)
				continue
			}
			req.Header.Add("Content-Type", "application/octet-stream")
			req.Header.Add("Ocp-Apim-Subscription-Key", key)
			resp, err := client.Do(req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "HTTP request for %s failed: %v", filename, err)
				continue
			}
			respJson := make(map[string]interface{})
			err = json.NewDecoder(resp.Body).Decode(&respJson)
			resp.Body.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "HTTP request for %s failed: %v", filename, err)
				continue
			}
			txt, err := json.MarshalIndent(respJson, "", "  ")
			if err != nil {
				fmt.Printf("%s: %s\n", filename, respJson)
			} else {
				fmt.Printf("%s: %s\n", filename, txt)
			}
		}
	}
}

func mainGoogle(verbose bool) {
	ctx := context.Background()
	client, err := google.DefaultClient(ctx, vision.CloudPlatformScope)
	if err != nil {
		log.Fatal(err)
	}
	service, err := vision.New(client)
	if err != nil {
		log.Fatal(err)
	}
	var (
		request      = &vision.BatchAnnotateImagesRequest{}
		requestSize  = 0
		requestFiles []string
	)
	for _, pattern := range flag.Args() {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid file pattern %s: %v", pattern, err)
			continue
		}
		for _, filename := range matches {
			byts, err := loadFile(filename)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Unable to load %s: %v\n", filename, err)
				continue
			}
			// 8 MB per request size limit as per:
			// https://cloud.google.com/vision/docs/best-practices#file_sizes
			if requestSize+len(byts) > 8<<20 {
				executeRequest(service, request, requestFiles, verbose)
				request.Requests = nil
				requestSize = 0
				requestFiles = nil
			}
			request.Requests = append(request.Requests, &vision.AnnotateImageRequest{
				Image: &vision.Image{
					Content: base64.StdEncoding.EncodeToString(byts),
				},
				Features: []*vision.Feature{{Type: "LABEL_DETECTION"}},
			})
			requestSize += len(byts)
			requestFiles = append(requestFiles, filename)
		}
	}
	executeRequest(service, request, requestFiles, verbose)
}

func executeRequest(service *vision.Service, request *vision.BatchAnnotateImagesRequest, requestFiles []string, verbose bool) {
	response, err := service.Images.Annotate(request).Do()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cloud Vision API request failed: %v", err)
		return
	}
	if verbose {
		txt, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			log.Printf("%+v\n", response)
		} else {
			log.Printf("%s\n", txt)
		}
	}
	for i, r := range response.Responses {
		labels := entityAnnotationsByConfidence(r.LabelAnnotations)
		sort.Sort(labels)
		fmt.Printf("%s: %v\n", requestFiles[i], labels)
	}
}

type entityAnnotationsByConfidence []*vision.EntityAnnotation

func (l entityAnnotationsByConfidence) Len() int           { return len(l) }
func (l entityAnnotationsByConfidence) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l entityAnnotationsByConfidence) Less(i, j int) bool { return l[i].Confidence < l[j].Confidence }
func (l entityAnnotationsByConfidence) String() string {
	strs := make([]string, l.Len())
	for i, a := range l {
		strs[i] = a.Description
	}
	return fmt.Sprintf("%v", strs)
}

func loadFile(filename string) ([]byte, error) {
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("stat failed: %v", err)
	}
	if stat.Size() > (4 << 20) {
		return nil, fmt.Errorf("file size (%v MB) is larger than recommended size of 4 MB as per https://cloud.google.com/vision/docs/best-practices#file_sizes", (stat.Size()*1.)/(1<<20))
	}
	byts, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read failed: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(byts))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}
	x, y := img.Bounds().Dx(), img.Bounds().Dy()
	if x < 640 || x < 480 {
		return nil, fmt.Errorf("image size (%dx%d) is smaller than recommended minimum of 640x480 as per https://cloud.google.com/vision/docs/best-practices#image_sizing", x, y)
	}
	log.Printf("%s is %d bytes and %dx%d pixels", filename, stat.Size(), x, y)
	return byts, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <filename>\n", os.Args[0])
	flag.PrintDefaults()
}
