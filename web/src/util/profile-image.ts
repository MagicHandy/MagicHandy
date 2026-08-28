// Account and persona portraits share one browser-side normalization path.
// The server still decodes and bounds the JPEG before writing it.
export const PROFILE_IMAGE_MAX_EDGE = 640;
const PROFILE_IMAGE_QUALITY = 0.92;

function fitWithin(width: number, height: number, maxEdge: number) {
  const scale = Math.min(1, maxEdge / Math.max(width, height));
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}

function paint(source: CanvasImageSource, width: number, height: number): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("this browser could not prepare the image");
  context.imageSmoothingEnabled = true;
  context.imageSmoothingQuality = "high";
  context.drawImage(source, 0, 0, width, height);
  return canvas;
}

function stepDown(
  source: CanvasImageSource,
  sourceWidth: number,
  sourceHeight: number,
  targetWidth: number,
  targetHeight: number,
): HTMLCanvasElement {
  let width = sourceWidth;
  let height = sourceHeight;
  let current = source;
  while (width > targetWidth * 2 && height > targetHeight * 2) {
    width = Math.max(targetWidth, Math.floor(width / 2));
    height = Math.max(targetHeight, Math.floor(height / 2));
    current = paint(current, width, height);
  }
  return paint(current, targetWidth, targetHeight);
}

export async function resizeImageToJPEG(file: File): Promise<Blob> {
  let bitmap: ImageBitmap | null = null;
  let canvas: HTMLCanvasElement | null = null;
  try {
    if (typeof createImageBitmap === "function") {
      bitmap = await createImageBitmap(file).catch(() => null);
    }
    if (bitmap) {
      const target = fitWithin(bitmap.width, bitmap.height, PROFILE_IMAGE_MAX_EDGE);
      if (target.width !== bitmap.width || target.height !== bitmap.height) {
        const resized = await createImageBitmap(file, {
          resizeWidth: target.width,
          resizeHeight: target.height,
          resizeQuality: "high",
        }).catch(() => null);
        if (resized && resized.width === target.width && resized.height === target.height) {
          bitmap.close();
          bitmap = resized;
          canvas = paint(bitmap, target.width, target.height);
        } else {
          resized?.close();
          canvas = stepDown(bitmap, bitmap.width, bitmap.height, target.width, target.height);
        }
      } else {
        canvas = paint(bitmap, target.width, target.height);
      }
    } else {
      const url = URL.createObjectURL(file);
      try {
        const image = await new Promise<HTMLImageElement>((resolve, reject) => {
          const element = new Image();
          element.onload = () => resolve(element);
          element.onerror = () => reject(new Error("that file could not be read as an image"));
          element.src = url;
        });
        const target = fitWithin(image.naturalWidth, image.naturalHeight, PROFILE_IMAGE_MAX_EDGE);
        canvas = stepDown(image, image.naturalWidth, image.naturalHeight, target.width, target.height);
      } finally {
        URL.revokeObjectURL(url);
      }
    }
    const blob = await new Promise<Blob | null>((resolve) => {
      canvas?.toBlob(resolve, "image/jpeg", PROFILE_IMAGE_QUALITY);
    });
    if (!blob) throw new Error("the image could not be encoded");
    return blob;
  } finally {
    bitmap?.close();
  }
}
