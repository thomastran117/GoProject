import { useState, useRef } from 'react'
import { requestUploadURL, uploadToAzure, confirmUpload } from '../api/blob'
import { createSubmission, getMySubmission, type SubmissionResponse } from '../api/submissions'

type UploadState = 'idle' | 'uploading' | 'success' | 'error'

interface Props {
  assignmentId: number
  token: string
}

export default function SubmissionUpload({ assignmentId, token }: Props) {
  const [state, setState] = useState<UploadState>('idle')
  const [message, setMessage] = useState('')
  const [submission, setSubmission] = useState<SubmissionResponse | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const file = fileRef.current?.files?.[0]
    if (!file) return

    setState('uploading')
    setMessage('')
    setSubmission(null)

    try {
      // Step 1: get SAS URL from backend
      const { upload_url, blob_key } = await requestUploadURL('submissions', token)

      // Step 2: PUT file directly to Azure
      await uploadToAzure(upload_url, file)

      // Step 3: confirm upload with backend
      await confirmUpload(blob_key, token)

      // Step 4: create submission record
      const result = await createSubmission(assignmentId, blob_key, file.name, token)
      setSubmission(result)
      setState('success')
      setMessage('Submission uploaded successfully.')
    } catch (err) {
      setState('error')
      setMessage(err instanceof Error ? err.message : 'An unexpected error occurred.')
    }
  }

  async function handleFetchMine() {
    setMessage('')
    setSubmission(null)
    try {
      const result = await getMySubmission(assignmentId, token)
      setSubmission(result)
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Failed to fetch submission.')
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <label className="text-sm font-medium text-gray-700">
          Upload your submission
          <input
            ref={fileRef}
            type="file"
            className="mt-1 block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-semibold file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
            disabled={state === 'uploading'}
          />
        </label>

        <button
          type="submit"
          disabled={state === 'uploading' || !token}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {state === 'uploading' ? 'Uploading…' : 'Submit Assignment'}
        </button>
      </form>

      <button
        onClick={handleFetchMine}
        disabled={!token}
        className="rounded border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-50"
      >
        View My Submission
      </button>

      {state === 'success' && (
        <p className="text-sm text-green-600">{message}</p>
      )}
      {state === 'error' && (
        <p className="text-sm text-red-600">{message}</p>
      )}
      {message && state !== 'success' && state !== 'error' && (
        <p className="text-sm text-red-600">{message}</p>
      )}

      {submission && (
        <div className="rounded border border-gray-200 bg-gray-50 p-4 text-sm space-y-1">
          <p><span className="font-medium">File:</span> {submission.file_name}</p>
          <p><span className="font-medium">Status:</span> {submission.status}</p>
          {submission.grade != null && (
            <p><span className="font-medium">Grade:</span> {submission.grade}</p>
          )}
          {submission.feedback && (
            <p><span className="font-medium">Feedback:</span> {submission.feedback}</p>
          )}
          <p><span className="font-medium">Submitted:</span> {new Date(submission.submitted_at).toLocaleString()}</p>
        </div>
      )}
    </div>
  )
}
