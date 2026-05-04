# Image-to-Image Generation Design

## Goal

Add an image-to-image flow so authenticated users can upload one reference image, provide a prompt, and generate a new image. Text-to-image remains the default workflow.

## Confirmed Scope

- Add a mode switch on the workspace: text-to-image and image-to-image.
- Image-to-image accepts exactly one reference image.
- Accepted upload types: PNG and JPEG.
- Maximum upload size: 5 MB.
- Image-to-image costs 1 credit, the same as text-to-image.
- Failed image-to-image tasks follow existing refund behavior.
- User history does not need to label image-to-image tasks for this version.
- Generated images continue to be stored and served through the existing private image endpoint.

## Out of Scope

- Multiple reference images.
- Masked or local-region editing.
- History badges or reference image display in history.
- Separate pricing or credit rules.
- Public access to uploaded reference images.

## User Experience

The workspace defaults to text-to-image. A mode control lets users switch to image-to-image. In image-to-image mode, the form shows an upload area before the existing prompt and ratio controls. Users can choose a PNG or JPEG image up to 5 MB, preview it, see the file name, and remove it before submitting.

Submit validation in image-to-image mode requires both a valid prompt and a valid reference image. Text-to-image keeps the current prompt-only behavior. The generated task progress, cancel behavior, preview dialog, download behavior, and credit refresh behavior remain unchanged.

## API Design

`POST /api/generations` supports two request shapes:

1. Existing text-to-image JSON request:
   - `Content-Type: application/json`
   - Body: `{ "prompt": string, "ratio": string }`

2. New image-to-image multipart request:
   - `Content-Type: multipart/form-data`
   - Fields: `prompt`, `ratio`, `reference_image`

The response remains `{ "task": ... }`. Existing task response fields remain stable. The API validates multipart uploads server-side, rejecting unsupported media types, empty files, and files larger than 5 MB.

## Persistence

Add a nullable `reference_image_path` column to `generation_tasks`. Text-to-image tasks leave it empty. Image-to-image tasks store the private path of the uploaded reference image.

Reference images are saved under the existing image storage root in a separate private namespace from generated output images. They are not exposed through a user-facing endpoint and are not returned by user history or admin generation audit responses.

## Worker Flow

Worker task claiming includes `reference_image_path`. If the task has no reference image, the worker calls the existing upstream generation flow. If the task has a reference image, the worker opens the private reference file and calls an upstream image edit flow using the same model, prompt, size, and PNG output settings.

The worker stores the generated output image through the existing storage path and marks success exactly as it does today. Upstream failures, timeout classification, storage failures, and refunds continue through the existing failure path.

## Upstream Client

Add an upstream method for image editing. It sends a multipart request to `/v1/images/edits` with model, prompt, size, `n=1`, quality `auto`, output format `png`, and the reference image file. Response decoding and error classification reuse the existing image generation logic.

## Error Handling

Client-side validation gives immediate messages for missing file, unsupported type, and file too large. Server-side validation is authoritative and returns safe Chinese error messages. The API must not log uploaded image bytes, API keys, prompts, or private paths in user-visible responses.

## Testing

Backend tests cover multipart creation, upload validation, reference image persistence, worker dispatch to edit vs generation, and upstream edit request shape. Frontend tests cover mode switching, upload validation, multipart submission, and existing text-to-image submission compatibility.

