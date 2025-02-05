
# Running

This makes a OpenAI compatible API for google vertex AI. This overrides the model to `google/gemini-2.0-flash-001` to fool any clients. Makes vertex AI work with most 
openai compatible clients.

``` shell
export GOOGLE_CLOUD_PROJECT=your-project-id
gcloud auth login
gcloud auth application-default login
go run main.go
```

