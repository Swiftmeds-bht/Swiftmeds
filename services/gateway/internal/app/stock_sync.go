package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	authv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/auth/v1"
	stocksyncv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/stock-sync/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// MaxStockCSVBytes is the Stock Sync file limit.
	MaxStockCSVBytes = 1 << 20
	// maxStockUploadBytes leaves room for the multipart headers around the file.
	maxStockUploadBytes = MaxStockCSVBytes + 64<<10
	// StockSyncCallTimeout bounds an upload: Stock Sync parses the whole file and
	// calls Inventory and Pricing before it answers, which takes longer than an
	// ordinary request.
	StockSyncCallTimeout = 10 * time.Second
)

var errCSVTooLarge = errors.New("the file is larger than 1 MB")

// stockSyncHandler serves the pharmacy dashboard's CSV upload and its import
// report (SCRUM-70). Like the stock routes it only serves the caller's own pharmacy.
func stockSyncHandler(client ports.StockSyncClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !ownPharmacy(w, r, identity) {
			return
		}
		pharmacyID := r.PathValue("pharmacyId")
		jobID := r.PathValue("jobId")

		switch {
		case r.Method == http.MethodPost && jobID == "":
			if identity.Role != authv1.Role_ROLE_PHARMACY_STAFF {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			filename, content, err := readStockCSV(w, r)
			if errors.Is(err, errCSVTooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), StockSyncCallTimeout)
			defer cancel()
			resp, err := client.UploadStockCsv(ctx, &stocksyncv1.UploadStockCsvRequest{
				PharmacyId:       pharmacyID,
				UploadedByUserId: identity.UserID,
				Filename:         filename,
				CsvContent:       content,
			})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusCreated, resp.GetJob())
		case r.Method == http.MethodGet && jobID != "":
			ctx, cancel := callContext(r)
			defer cancel()
			resp, err := client.GetSyncJob(ctx, &stocksyncv1.GetSyncJobRequest{JobId: jobID})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			// GetSyncJob looks a job up by ID alone, so check the owner here. Another
			// pharmacy's job answers 404 so job IDs cannot be probed.
			if resp.GetJob().GetPharmacyId() != pharmacyID {
				writeDownstreamError(w, logger, r, status.Error(codes.NotFound, "sync job not found"))
				return
			}
			writeProtoStatus(w, http.StatusOK, resp.GetJob())
		case r.Method == http.MethodGet:
			page, ok := pageRequest(r)
			if !ok {
				writeError(w, http.StatusBadRequest, "invalid sync job list parameters")
				return
			}
			ctx, cancel := callContext(r)
			defer cancel()
			resp, err := client.ListSyncJobs(ctx, &stocksyncv1.ListSyncJobsRequest{PharmacyId: pharmacyID, Page: page})
			if err != nil {
				writeDownstreamError(w, logger, r, err)
				return
			}
			writeProtoStatus(w, http.StatusOK, resp)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// readStockCSV streams the multipart body and returns the "file" part. The
// whole body is capped, so an oversized upload is refused without reading it all.
func readStockCSV(w http.ResponseWriter, r *http.Request) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxStockUploadBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		return "", nil, errors.New("the request must be multipart/form-data with a file part")
	}

	var (
		filename string
		content  []byte
		found    bool
	)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, uploadReadError(err)
		}
		if part.FormName() != "file" || found {
			if _, err := io.Copy(io.Discard, part); err != nil {
				return "", nil, uploadReadError(err)
			}
			continue
		}
		content, err = io.ReadAll(io.LimitReader(part, MaxStockCSVBytes+1))
		if err != nil {
			return "", nil, uploadReadError(err)
		}
		if len(content) > MaxStockCSVBytes {
			return "", nil, errCSVTooLarge
		}
		filename = cleanFilename(part.FileName())
		found = true
	}

	if !found {
		return "", nil, errors.New("the file part is missing")
	}
	if !utf8.Valid(content) {
		return "", nil, errors.New("the file is not valid UTF-8")
	}
	return filename, content, nil
}

func uploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errCSVTooLarge
	}
	return errors.New("the upload could not be read")
}

// cleanFilename keeps only the base name the browser sent, never a client path.
func cleanFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" {
		return "upload.csv"
	}
	return name
}
