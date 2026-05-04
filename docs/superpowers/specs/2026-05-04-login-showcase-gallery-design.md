# Login Showcase Gallery Design

## Goal

Add a pure-image showcase to the unauthenticated login/home screen so new users can quickly see the quality and range of generated images before logging in.

## Scope

- Show exactly six showcase images on the unauthenticated login page.
- Use the side-by-side layout selected during brainstorming: login form on the left, showcase image grid on the right.
- Do not show prompts, tags, captions, ratios, explanations, or image metadata.
- Keep authenticated pages unchanged.

## Source Images

The user provided six PNG files in the project `pic/` directory:

- `pic/罗威纳.png`
- `pic/伯恩山.png`
- `pic/恭王府.png`
- `pic/陈平安.png`
- `pic/左右.png`
- `pic/起床.png`

Implementation should make these images available to the Vite app from a browser-readable static asset location, such as `web/public/showcase/`, and reference them from a fixed frontend array.

## UI Design

The unauthenticated login screen becomes a two-column composition:

- Left column: the existing login card and register action.
- Right column: a six-image gallery using a compact grid.

The gallery should match the current jade, gold, and mist visual language without adding marketing copy. Images should be the primary signal. Cards may use restrained borders, existing shadows, and 8-12px radii consistent with the current app.

## Responsive Behavior

- Desktop/tablet: login card and gallery sit side by side.
- Mobile: stack vertically with login first, gallery second.
- The six images should stay visually even by using a stable tile ratio and `object-fit: cover`.
- The portrait image `pic/起床.png` should crop gracefully inside the same tile treatment.

## Accessibility

- The gallery should have an accessible section label.
- Each image should have short descriptive alt text based on the image filename.
- The images are decorative proof-of-quality content and should not be interactive.

## Testing

- Add or update frontend tests to confirm unauthenticated users see the showcase images on the login page.
- Confirm the authenticated workspace remains the restored-session destination and does not render the login showcase.
- Run the existing frontend test suite after implementation.
