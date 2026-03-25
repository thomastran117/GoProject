package blob

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"backend/internal/config/middleware"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/google/uuid"
)

// UploadURLResponse is returned by GenerateUploadURL.
// UploadURL is the short-lived SAS URL the client uses to PUT the file directly
// to Azure. BlobKey is the blob path to persist in the database after the
// upload is confirmed via ConfirmUpload.
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	BlobKey   string `json:"blob_key"`
}

// Service handles Azure Blob Storage operations.
type Service struct {
	credential    *azblob.SharedKeyCredential
	serviceClient *service.Client
	accountName   string
	containerName string
}

// NewService creates a Service using shared key authentication.
func NewService(accountName, accountKey, containerName string) (*Service, error) {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", accountName)
	svcClient, err := service.NewClientWithSharedKeyCredential(serviceURL, credential, nil)
	if err != nil {
		return nil, err
	}

	return &Service{
		credential:    credential,
		serviceClient: svcClient,
		accountName:   accountName,
		containerName: containerName,
	}, nil
}

// GenerateUploadURL creates a 15-minute SAS URL that allows a single PUT to a
// new blob. folder is an optional path prefix (e.g. "avatars"). Returns the
// upload URL and the blob key. The blob key must be confirmed via ConfirmUpload
// before being stored in the database.
func (s *Service) GenerateUploadURL(ctx context.Context, folder string) (*UploadURLResponse, error) {
	blobName := uuid.New().String()
	if folder != "" {
		blobName = folder + "/" + blobName
	}

	perms := sas.BlobPermissions{Write: true, Create: true}
	now := time.Now().UTC()
	sasParams, err := sas.BlobSignatureValues{
		Protocol:      sas.ProtocolHTTPS,
		StartTime:     now.Add(-5 * time.Minute), // clock-skew tolerance
		ExpiryTime:    now.Add(15 * time.Minute),
		Permissions:   perms.String(),
		ContainerName: s.containerName,
		BlobName:      blobName,
	}.SignWithSharedKey(s.credential)
	if err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusInternalServerError,
			Code:    "BLOB_SAS_ERROR",
			Message: "Failed to generate upload URL",
		}
	}

	uploadURL := fmt.Sprintf(
		"https://%s.blob.core.windows.net/%s/%s?%s",
		s.accountName, s.containerName, blobName, sasParams.Encode(),
	)

	return &UploadURLResponse{
		UploadURL: uploadURL,
		BlobKey:   blobName,
	}, nil
}

// ConfirmUpload verifies that the blob identified by blobKey actually exists in
// Azure. Call this after the client reports a successful upload. Returns nil
// when the blob is present, or a 404 APIError when it is not.
func (s *Service) ConfirmUpload(ctx context.Context, blobKey string) error {
	blobClient := s.serviceClient.NewContainerClient(s.containerName).NewBlobClient(blobKey)

	_, err := blobClient.GetProperties(ctx, nil)
	if err == nil {
		return nil
	}

	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return &middleware.APIError{
			Status:  http.StatusNotFound,
			Code:    "BLOB_NOT_FOUND",
			Message: "Blob not found or upload incomplete",
		}
	}

	return &middleware.APIError{
		Status:  http.StatusInternalServerError,
		Code:    "BLOB_CHECK_ERROR",
		Message: "Failed to verify upload",
	}
}
