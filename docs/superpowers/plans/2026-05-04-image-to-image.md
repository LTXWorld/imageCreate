# Image-to-Image Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add authenticated image-to-image generation with one uploaded PNG/JPEG reference image, mode switching in the workspace, and existing credit/refund behavior.

**Architecture:** Keep text-to-image JSON generation unchanged. Use multipart `POST /api/generations` only for image-to-image, persist the private reference image path on the task, and have the worker route tasks with a reference image to an upstream image-edit request. User task responses and history stay backward compatible.

**Tech Stack:** Go `net/http`, chi, pgx migrations, existing file storage, OpenAI-compatible Images API, React 18, Vite, Vitest, Testing Library.

---

## File Structure

- `api/migrations/000003_reference_images.up.sql`: adds nullable `reference_image_path` to `generation_tasks`.
- `api/internal/generations/storage.go`: adds private reference-image save/open helpers using existing storage root.
- `api/internal/generations/service.go`: carries `ReferenceImagePath` through `CreateTaskInput`, `Task`, inserts, selects, and task scanning.
- `api/internal/generations/handlers.go`: accepts JSON and multipart creation requests; validates PNG/JPEG and 5 MB limit.
- `api/internal/worker/worker.go`: claims reference image path and dispatches generation vs edit upstream calls.
- `api/internal/upstream/client.go`: adds multipart `/images/edits` support while reusing response/error handling.
- Existing backend tests: add focused tests in adjacent `*_test.go` files.
- `web/src/api/client.ts`: adds multipart generation API path and keeps JSON path unchanged.
- `web/src/pages/WorkspacePage.tsx`: adds mode switch, upload validation, preview, and multipart submit.
- `web/src/pages/WorkspacePage.test.tsx`: adds UI and submit tests; preserves existing text-to-image behavior.
- `web/src/styles/app.css`: styles the mode switch and upload panel.

## Task 1: Persist Reference Image Paths

**Files:**
- Create: `api/migrations/000003_reference_images.up.sql`
- Modify: `api/internal/generations/service.go`
- Test: `api/internal/generations/service_test.go`

- [ ] **Step 1: Write the failing service test**

Add this test to `api/internal/generations/service_test.go`:

```go
func TestCreateTaskPersistsReferenceImagePath(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "service-reference-image", 2)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID:             userID,
		Prompt:             "把参考图变成电影海报",
		Ratio:              "1:1",
		ReferenceImagePath: "references/task-reference.jpg",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	got, err := service.GetTaskForUser(ctx, userID, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ReferenceImagePath != "references/task-reference.jpg" {
		t.Fatalf("reference image path = %q, want references/task-reference.jpg", got.ReferenceImagePath)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generations -run TestCreateTaskPersistsReferenceImagePath -count=1`

Expected: compile failure because `CreateTaskInput.ReferenceImagePath` and `Task.ReferenceImagePath` do not exist.

- [ ] **Step 3: Add migration**

Create `api/migrations/000003_reference_images.up.sql`:

```sql
ALTER TABLE generation_tasks
ADD COLUMN reference_image_path TEXT;
```

- [ ] **Step 4: Add service fields and SQL plumbing**

In `api/internal/generations/service.go`, add `ReferenceImagePath string` to `CreateTaskInput` and `Task`. Update `taskSelectSQL` to select `COALESCE(reference_image_path, '')` after `COALESCE(image_path, '')`. Update `scanTask` to scan into `task.ReferenceImagePath`. Change `insertTask` to accept `referenceImagePath string`, insert `reference_image_path`, and return it.

Use this signature:

```go
func insertTask(ctx context.Context, tx pgx.Tx, userID, prompt, size, model, referenceImagePath string) (Task, error)
```

Change the `CreateTask` call to:

```go
task, err := insertTask(ctx, tx, input.UserID, prompt, size, s.Model, input.ReferenceImagePath)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/generations -run TestCreateTaskPersistsReferenceImagePath -count=1`

Expected: PASS.

## Task 2: Store and Validate Uploaded Reference Images

**Files:**
- Modify: `api/internal/generations/storage.go`
- Modify: `api/internal/generations/handlers.go`
- Test: `api/internal/generations/handlers_test.go`

- [ ] **Step 1: Write failing multipart creation test**

Add a helper and test to `api/internal/generations/handlers_test.go`:

```go
func authenticatedMultipartRequest(t *testing.T, method, target string, fields map[string]string, fileField, fileName string, fileBytes []byte, userID string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := authenticatedJSONRequest(t, method, target, "", userID)
	req.Body = io.NopCloser(&body)
	req.ContentLength = int64(body.Len())
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestCreateGenerationAcceptsReferenceImageUpload(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-reference-upload", 3)
	pngBytes := []byte("\x89PNG\r\n\x1a\nreference-bytes")

	req := authenticatedMultipartRequest(t, http.MethodPost, "/api/generations", map[string]string{
		"prompt": "把参考图变成电影海报",
		"ratio":  "1:1",
	}, "reference_image", "reference.png", pngBytes, userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var taskID, referencePath string
	if err := db.QueryRow(ctx, `
		SELECT id::text, COALESCE(reference_image_path, '')
		FROM generation_tasks
		WHERE user_id = $1::uuid
	`, userID).Scan(&taskID, &referencePath); err != nil {
		t.Fatalf("select task: %v", err)
	}
	if referencePath == "" {
		t.Fatal("reference image path is empty")
	}
	if strings.Contains(rec.Body.String(), referencePath) {
		t.Fatalf("response leaked reference image path: %s", rec.Body.String())
	}
}
```

Also add imports: `bytes`, `io`, and `mime/multipart`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generations -run TestCreateGenerationAcceptsReferenceImageUpload -count=1`

Expected: FAIL because handler only decodes JSON and does not save reference images.

- [ ] **Step 3: Add storage helper**

In `api/internal/generations/storage.go`, add:

```go
func (s ImageStorage) SaveReference(ctx context.Context, taskSeed string, data []byte, extension string, now time.Time) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if invalidPathPart(taskSeed) {
		return "", errors.New("invalid reference seed")
	}
	if extension != ".png" && extension != ".jpg" && extension != ".jpeg" {
		return "", errors.New("invalid reference extension")
	}

	relativePath := filepath.Join(
		"references",
		fmt.Sprintf("%04d", now.Year()),
		fmt.Sprintf("%02d", int(now.Month())),
		fmt.Sprintf("%02d", now.Day()),
		taskSeed+extension,
	)
	fullPath := s.fullPath(relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("create reference image directory: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write reference image: %w", err)
	}
	return relativePath, nil
}
```

- [ ] **Step 4: Add multipart handler path**

In `api/internal/generations/handlers.go`, add constants:

```go
const maxReferenceImageBytes = 5 << 20
```

Update `Create` to branch on `strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")`. For multipart, call `r.ParseMultipartForm(maxReferenceImageBytes)`, read `prompt`, `ratio`, and `reference_image`, limit the file with `io.LimitReader(file, maxReferenceImageBytes+1)`, reject if too large, validate extension and sniffed content type, save via `h.Storage.SaveReference` using a random seed such as `uuid.NewString()`, then call `CreateTask` with `ReferenceImagePath`.

Use messages:

```go
writeError(w, http.StatusBadRequest, "invalid_reference_image", "请上传 PNG 或 JPEG 图片")
writeError(w, http.StatusBadRequest, "reference_image_too_large", "参考图不能超过 5MB")
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/generations -run TestCreateGenerationAcceptsReferenceImageUpload -count=1`

Expected: PASS.

## Task 3: Add Upstream Image Edit Support

**Files:**
- Modify: `api/internal/upstream/client.go`
- Test: `api/internal/upstream/client_test.go`

- [ ] **Step 1: Write failing upstream edit request test**

Add this test to `api/internal/upstream/client_test.go`:

```go
func TestEditImageSendsMultipartRequest(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotContentType string
	gotFields := map[string]string{}
	var gotFileName string
	var gotFileBytes []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			if part.FormName() == "image" {
				gotFileName = part.FileName()
				gotFileBytes = data
				continue
			}
			gotFields[part.FormName()] = string(data)
		}
		w.Header().Set("X-Request-Id", "req-edit-123")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"ZWRpdGVkLWJ5dGVz"}]}`))
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, APIKey: "test-api-key", Model: "gpt-image-2", HTTPClient: server.Client()}
	result, err := client.EditImage(context.Background(), "make it cinematic", "1024x1024", "reference.png", []byte("reference-bytes"))
	if err != nil {
		t.Fatalf("edit image: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/images/edits" {
		t.Fatalf("path = %q, want /v1/images/edits", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Fatalf("authorization = %q, want bearer API key", gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Fatalf("content-type = %q, want multipart", gotContentType)
	}
	wantFields := map[string]string{"model": "gpt-image-2", "prompt": "make it cinematic", "n": "1", "size": "1024x1024", "quality": "auto", "output_format": "png"}
	for key, want := range wantFields {
		if gotFields[key] != want {
			t.Fatalf("field %s = %q, want %q", key, gotFields[key], want)
		}
	}
	if gotFileName != "reference.png" {
		t.Fatalf("file name = %q, want reference.png", gotFileName)
	}
	if string(gotFileBytes) != "reference-bytes" {
		t.Fatalf("file bytes = %q, want reference-bytes", string(gotFileBytes))
	}
	if string(result.ImageBytes) != "edited-bytes" {
		t.Fatalf("image bytes = %q, want edited-bytes", string(result.ImageBytes))
	}
}
```

Add import `io`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/upstream -run TestEditImageSendsMultipartRequest -count=1`

Expected: compile failure because `Client.EditImage` does not exist.

- [ ] **Step 3: Implement edit request**

In `api/internal/upstream/client.go`, add `mime/multipart` import and implement:

```go
func (c Client) EditImage(ctx context.Context, prompt, size, fileName string, imageBytes []byte) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult("timeout", sanitizedMessage("timeout")), ErrTimeout
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":         c.Model,
		"prompt":        prompt,
		"n":             "1",
		"size":          size,
		"quality":       "auto",
		"output_format": "png",
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return errorResult("upstream_error", "encode upstream request"), fmt.Errorf("%w: encode request", ErrUpstream)
		}
	}
	part, err := writer.CreateFormFile("image", fileName)
	if err != nil {
		return errorResult("upstream_error", "encode upstream request"), fmt.Errorf("%w: encode request", ErrUpstream)
	}
	if _, err := part.Write(imageBytes); err != nil {
		return errorResult("upstream_error", "encode upstream request"), fmt.Errorf("%w: encode request", ErrUpstream)
	}
	if err := writer.Close(); err != nil {
		return errorResult("upstream_error", "encode upstream request"), fmt.Errorf("%w: encode request", ErrUpstream)
	}

	return c.doImageRequest(ctx, imageEditEndpoint(c.BaseURL), writer.FormDataContentType(), &body, size)
}
```

Refactor `GenerateImage` so after JSON encoding it calls a shared `doImageRequest` helper containing the existing HTTP send, response decode, and error mapping. Add:

```go
func imageEditEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/images/edits"
	}
	return base + "/v1/images/edits"
}
```

- [ ] **Step 4: Run upstream tests**

Run: `go test ./internal/upstream -count=1`

Expected: PASS.

## Task 4: Route Worker Tasks to Edit or Generate

**Files:**
- Modify: `api/internal/worker/worker.go`
- Test: `api/internal/worker/worker_test.go`

- [ ] **Step 1: Write failing worker dispatch test**

Add this test to `api/internal/worker/worker_test.go` using existing worker test helpers:

```go
func TestProcessOneUsesEditForReferenceImageTask(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	storage := generations.ImageStorage{Root: t.TempDir()}
	service := testGenerationService(db)
	userID := insertWorkerTestUser(t, ctx, db, "worker-edit-user", 2)
	referencePath, err := storage.SaveReference(ctx, "worker-reference", []byte("reference-bytes"), ".png", time.Now())
	if err != nil {
		t.Fatalf("save reference: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, reference_image_path, created_at)
		VALUES ($1::uuid, 'make cinematic', '1024x1024', $2, 'test-image-model', $3, now() - interval '1 minute')
		RETURNING id::text
	`, userID, models.TaskQueued, referencePath).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	upstream := &recordingWorkerUpstream{editResult: upstream.Result{RequestID: "req-edit-worker", ImageBytes: []byte("edited-output")}}
	worker := Worker{DB: db, Generations: service, Upstream: upstream, Storage: storage, ClaimDelay: -time.Second}

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if upstream.generateCalls != 0 {
		t.Fatalf("generate calls = %d, want 0", upstream.generateCalls)
	}
	if upstream.editCalls != 1 {
		t.Fatalf("edit calls = %d, want 1", upstream.editCalls)
	}
	if upstream.editPrompt != "make cinematic" || upstream.editSize != "1024x1024" {
		t.Fatalf("edit prompt/size = %q/%q", upstream.editPrompt, upstream.editSize)
	}
	if string(upstream.editImageBytes) != "reference-bytes" {
		t.Fatalf("edit image bytes = %q, want reference-bytes", string(upstream.editImageBytes))
	}
}
```

Extend the existing fake upstream or add:

```go
type recordingWorkerUpstream struct {
	generateCalls int
	editCalls     int
	editPrompt    string
	editSize      string
	editImageBytes []byte
	editResult    upstream.Result
}

func (u *recordingWorkerUpstream) GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error) {
	u.generateCalls++
	return upstream.Result{RequestID: "req-generate", ImageBytes: []byte("generated-output")}, nil
}

func (u *recordingWorkerUpstream) EditImage(ctx context.Context, prompt, size, fileName string, imageBytes []byte) (upstream.Result, error) {
	u.editCalls++
	u.editPrompt = prompt
	u.editSize = size
	u.editImageBytes = imageBytes
	return u.editResult, nil
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/worker -run TestProcessOneUsesEditForReferenceImageTask -count=1`

Expected: compile failure because worker `Upstream` lacks `EditImage` and claimed tasks lack reference image paths.

- [ ] **Step 3: Implement worker dispatch**

In `api/internal/worker/worker.go`, change `Upstream` to:

```go
type Upstream interface {
	GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error)
	EditImage(ctx context.Context, prompt, size, fileName string, imageBytes []byte) (upstream.Result, error)
}
```

Change `claimedTask` to include `referenceImagePath string`. Change claim SQL to select `COALESCE(reference_image_path, '')`.

In `ProcessOne`, replace the direct generate call with:

```go
var result upstream.Result
var err error
if task.referenceImagePath == "" {
	result, err = w.Upstream.GenerateImage(ctx, task.prompt, task.size)
} else {
	file, openErr := w.Storage.Open(ctx, task.referenceImagePath)
	if openErr != nil {
		return true, w.Generations.MarkFailedAndRefund(ctx, task.id, storageErrorCode, "failed to read reference image", 0)
	}
	imageBytes, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return true, w.Generations.MarkFailedAndRefund(ctx, task.id, storageErrorCode, "failed to read reference image", 0)
	}
	result, err = w.Upstream.EditImage(ctx, task.prompt, task.size, filepath.Base(task.referenceImagePath), imageBytes)
}
```

Add imports `io` and `path/filepath`.

- [ ] **Step 4: Run worker tests**

Run: `go test ./internal/worker -count=1`

Expected: PASS.

## Task 5: Add Frontend API Multipart Support

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.test.ts`

- [ ] **Step 1: Write failing API client test**

Add this test to `web/src/api/client.test.ts`:

```ts
test("createGeneration sends multipart form data when reference image is provided", async () => {
  const file = new File(["reference-bytes"], "reference.png", { type: "image/png" });
  const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
    jsonResponse({
      task: {
        id: "task-reference",
        prompt: "电影海报",
        ratio: "1:1",
        size: "1024x1024",
        status: "queued",
        created_at: "2026-04-30T08:00:00Z",
      },
    }),
  );

  await api.createGeneration({ prompt: "电影海报", ratio: "1:1", referenceImage: file });

  const [, init] = fetchMock.mock.calls[0];
  expect(init).toMatchObject({ method: "POST", credentials: "include" });
  expect(init?.headers).toBeUndefined();
  expect(init?.body).toBeInstanceOf(FormData);
  const body = init?.body as FormData;
  expect(body.get("prompt")).toBe("电影海报");
  expect(body.get("ratio")).toBe("1:1");
  expect(body.get("reference_image")).toBe(file);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm test -- --run src/api/client.test.ts`

Expected: compile or assertion failure because `referenceImage` is not supported.

- [ ] **Step 3: Implement API client support**

In `web/src/api/client.ts`, change the create generation input type to include `referenceImage?: File`. In `createGeneration`, if `input.referenceImage` exists, build `FormData`, append `prompt`, `ratio`, and `reference_image`, and call `request` without JSON headers. Otherwise keep the existing JSON behavior unchanged.

- [ ] **Step 4: Run API client tests**

Run: `npm test -- --run src/api/client.test.ts`

Expected: PASS.

## Task 6: Add Workspace Mode Switch and Upload UI

**Files:**
- Modify: `web/src/pages/WorkspacePage.tsx`
- Modify: `web/src/styles/app.css`
- Test: `web/src/pages/WorkspacePage.test.tsx`

- [ ] **Step 1: Write failing workspace multipart submit test**

Add this test to `web/src/pages/WorkspacePage.test.tsx`:

```tsx
test("submits reference image in image-to-image mode", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() =>
    jsonResponse({
      task: {
        id: "task-reference-ui",
        prompt: "电影海报",
        ratio: "1:1",
        size: "1024x1024",
        status: "queued",
        created_at: "2026-04-30T08:00:00Z",
      },
    }),
  );

  render(<WorkspacePage user={user} />);

  await userEvent.click(screen.getByRole("button", { name: "图生图" }));
  const file = new File(["reference-bytes"], "reference.png", { type: "image/png" });
  await userEvent.upload(screen.getByLabelText("参考图"), file);
  await userEvent.type(screen.getByLabelText("提示词"), "电影海报");
  await userEvent.click(screen.getByRole("button", { name: "生成" }));

  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalled();
  });
  const [, init] = fetchMock.mock.calls[0];
  expect(init?.body).toBeInstanceOf(FormData);
  const body = init?.body as FormData;
  expect(body.get("prompt")).toBe("电影海报");
  expect(body.get("ratio")).toBe("1:1");
  expect(body.get("reference_image")).toBe(file);
});
```

- [ ] **Step 2: Write failing upload validation test**

Add this test to `web/src/pages/WorkspacePage.test.tsx`:

```tsx
test("rejects unsupported reference image type before submit", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(() => jsonResponse({}));

  render(<WorkspacePage user={user} />);

  await userEvent.click(screen.getByRole("button", { name: "图生图" }));
  const file = new File(["bad"], "reference.gif", { type: "image/gif" });
  await userEvent.upload(screen.getByLabelText("参考图"), file);
  await userEvent.type(screen.getByLabelText("提示词"), "电影海报");
  await userEvent.click(screen.getByRole("button", { name: "生成" }));

  expect(screen.getByRole("alert")).toHaveTextContent("请上传 PNG 或 JPEG 图片");
  expect(fetchMock).not.toHaveBeenCalled();
});
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `npm test -- --run src/pages/WorkspacePage.test.tsx`

Expected: FAIL because mode switch and upload input do not exist.

- [ ] **Step 4: Implement workspace state and submit logic**

In `web/src/pages/WorkspacePage.tsx`, add state:

```ts
type GenerationMode = "text" | "image";
const [generationMode, setGenerationMode] = useState<GenerationMode>("text");
const [referenceImage, setReferenceImage] = useState<File | null>(null);
const [referencePreviewURL, setReferencePreviewURL] = useState("");
```

Add constants:

```ts
const maxReferenceImageBytes = 5 * 1024 * 1024;
const acceptedReferenceImageTypes = new Set(["image/png", "image/jpeg"]);
```

In submit, if `generationMode === "image"`, require `referenceImage`, validate type and size, and call:

```ts
await api.createGeneration({ prompt: latestPrompt, ratio: selectedRatio, referenceImage });
```

Text mode continues calling without `referenceImage`.

- [ ] **Step 5: Add mode switch and upload UI**

Add buttons labelled `文生图` and `图生图`. In image mode render:

```tsx
<div className="reference-upload-panel">
  <label htmlFor="reference-image">参考图</label>
  <input id="reference-image" aria-label="参考图" type="file" accept="image/png,image/jpeg" onChange={handleReferenceImageChange} />
  {referencePreviewURL ? <img src={referencePreviewURL} alt="参考图预览" /> : <p>上传 PNG 或 JPEG，最大 5MB</p>}
  {referenceImage ? <button type="button" onClick={clearReferenceImage}>移除参考图</button> : null}
</div>
```

Use `URL.createObjectURL` for preview and revoke old URLs in cleanup.

- [ ] **Step 6: Add styles**

In `web/src/styles/app.css`, add styles for `.mode-switch`, `.reference-upload-panel`, `.reference-preview`, and `.reference-file-meta` consistent with existing form/card styles.

- [ ] **Step 7: Run workspace tests**

Run: `npm test -- --run src/pages/WorkspacePage.test.tsx`

Expected: PASS.

## Task 7: Full Verification

**Files:**
- No new files.

- [ ] **Step 1: Run backend tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 2: Run frontend tests**

Run: `npm test`

Expected: PASS.

- [ ] **Step 3: Run frontend build**

Run: `npm run build`

Expected: PASS with TypeScript and Vite build success.

- [ ] **Step 4: Manual smoke checklist**

Start the app locally and verify:

```text
1. Workspace defaults to 文生图.
2. 文生图 can submit without choosing a file.
3. Switching to 图生图 shows the reference image upload panel.
4. PNG/JPEG under 5 MB shows a preview and can be removed.
5. GIF or files over 5 MB show validation errors before submit.
6. 图生图 submit creates a queued task and refreshes credits.
```

---

## Self-Review

- Spec coverage: workspace mode switch, PNG/JPEG 5 MB validation, multipart API, private persistence, worker edit routing, upstream edit request, unchanged history response, credit/refund reuse, and tests are covered.
- Placeholder scan: no `TBD`, `TODO`, or unspecified implementation steps remain.
- Type consistency: `ReferenceImagePath`, `referenceImage`, and `reference_image` are used consistently across Go, TypeScript, and multipart fields.
