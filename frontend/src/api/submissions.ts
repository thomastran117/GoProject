const API_BASE = '/api'

export interface SubmissionResponse {
  id: number
  assignment_id: number
  student_id: number
  blob_key: string
  file_name: string
  status: 'submitted' | 'late' | 'graded'
  grade: number | null
  feedback: string
  submitted_at: string
  updated_at: string
}

export async function createSubmission(
  assignmentId: number,
  blobKey: string,
  fileName: string,
  token: string,
): Promise<SubmissionResponse> {
  const res = await fetch(`${API_BASE}/assignments/${assignmentId}/submissions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
    },
    body: JSON.stringify({ blob_key: blobKey, file_name: fileName }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message ?? `Submit failed: ${res.status}`)
  }
  const body = await res.json()
  return body.data as SubmissionResponse
}

export async function getMySubmission(
  assignmentId: number,
  token: string,
): Promise<SubmissionResponse> {
  const res = await fetch(`${API_BASE}/assignments/${assignmentId}/submissions/mine`, {
    headers: { 'Authorization': `Bearer ${token}` },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body?.error?.message ?? `Fetch failed: ${res.status}`)
  }
  const body = await res.json()
  return body.data as SubmissionResponse
}
