import { useEffect } from "react";

type ImagePreviewDialogProps = {
  alt: string;
  onClose: () => void;
  src: string;
};

export function ImagePreviewDialog({ alt, onClose, src }: ImagePreviewDialogProps) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div
      aria-label="图片预览"
      aria-modal="true"
      className="image-preview-overlay"
      onClick={onClose}
      role="dialog"
    >
      <div className="image-preview-dialog" onClick={(event) => event.stopPropagation()}>
        <button
          aria-label="关闭图片预览"
          className="image-preview-close"
          onClick={onClose}
          type="button"
        >
          ×
        </button>
        <img className="image-preview-large" src={src} alt={alt} />
      </div>
    </div>
  );
}
