
# [Google Cloud Vision API](https://cloud.google.com/vision/)

- [Setup the Cloud Vision API](https://cloud.google.com/vision/docs/quickstart#set_up_a_google_cloud_vision_api_project)
- Setup a service account and the GOOGLE_APPLICATION_CREDENTIALS environment variable (for [Application Default Credentials](https://cloud.google.com/vision/docs/auth-template/cloud-api-auth#authenticating_with_application_default_credentials))
- `go run main.go --api=google <filepattern of files to run the API on>`

# [Microsoft Cognitive Services Computer Vision API](https://www.microsoft.com/cognitive-services)

- [Setup the API](https://www.microsoft.com/cognitive-services)
- Set the MICROSOFT_API_KEY environment variable to the [key from the console](https://www.microsoft.com/cognitive-services/en-US/subscriptions)
- `go run main.go --api=microsoft <filepattern of files to run the API on>`
