const API_BASE = '/api'

export interface UploadURLResponse {
  upload_url: string
  blob_key: string
}

/**
 * Step 1: request a short-lived SAS upload URL from the backend.
 */
export async function requestUploadURL(folder: string, token: string): Promise<UploadURLResponse> {
  const res = await fetch(`${API_BASE}/blob/upload-url`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ folder }),
  })
  if (!res.ok) throw new Error(`Failed to get upload URL: ${res.status}`)
  const body = await res.json()
  return body.data as UploadURLResponse
}

/**
 * Step 2: PUT the file directly to Azure using the SAS URL.
 * The x-ms-blob-type header is required by Azure for BlockBlob uploads.
 */
export async function uploadToAzure(uploadUrl: string, file: File): Promise<void> {
  const res = await fetch(uploadUrl, {
    method: 'PUT',
    headers: {
      'x-ms-blob-type': 'BlockBlob',
      'Content-Type': file.type || 'application/octet-stream',
    },
    body: file,
  })
  if (!res.ok) throw new Error(`Azure upload failed: ${res.status}`)
}

/**
 * Step 3: confirm the upload with the backend so it can verify the blob exists.
 */
export async function confirmUpload(blobKey: string, token: string): Promise<void> {
  const res = await fetch(`${API_BASE}/blob/confirm`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ blob_key: blobKey }),
  })
  if (!res.ok) throw new Error(`Confirm upload failed: ${res.status}`)
}
