package app

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	analyticsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/analytics/v1"
	pharmacyregistryv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/pharmacy-registry/v1"
	"github.com/Swiftmeds-bht/Swiftmeds/services/gateway/internal/ports"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func unmetDemandHandler(
	analytics ports.AnalyticsClient,
	registry ports.PharmacyRegistryClient,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity := identityFrom(r)
		if !pharmacyOnly(w, identity) {
			return
		}
		if identity.PharmacyID == "" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		ctx, cancel := callContext(r)
		defer cancel()

		pharmacyResponse, err := registry.GetPharmacy(ctx, &pharmacyregistryv1.GetPharmacyRequest{
			PharmacyId: identity.PharmacyID,
		})
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}
		if pharmacyResponse == nil || pharmacyResponse.GetPharmacy() == nil {
			writeError(w, http.StatusBadGateway, "pharmacy registry returned no pharmacy")
			return
		}

		dzongkhag := strings.TrimSpace(pharmacyResponse.GetPharmacy().GetDzongkhag())
		if dzongkhag == "" {
			writeError(w, http.StatusConflict, "pharmacy dzongkhag is not configured")
			return
		}

		to := time.Now().UTC()
		from := to.Add(-7 * 24 * time.Hour)
		report, err := analytics.GetUnmetDemand(ctx, &analyticsv1.GetUnmetDemandRequest{
			Range: &analyticsv1.TimeRange{
				From: timestamppb.New(from),
				To:   timestamppb.New(to),
			},
			Dzongkhag: dzongkhag,
			Limit:     10,
		})
		if err != nil {
			writeDownstreamError(w, logger, r, err)
			return
		}

		writeProtoStatus(w, http.StatusOK, report)
	}
}
