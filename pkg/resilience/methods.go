package resilience

// The retry decision for every RPC in proto/ (SCRUM-132, follows SCRUM-55).
//
// UnaryClientInterceptor retries only the methods in retryableMethods. Every
// other method, including one that is in neither list, is called once, because
// a retry after codes.Unavailable may run a write a second time: the server can
// have done the work before the connection broke.
//
// TestEveryProtoRPCHasARetryDecision fails when an RPC in proto/ is in neither
// list (or in both), so a new RPC cannot ship without a decision. To decide:
//
//   - retryableMethods: the RPC only reads, or the proto marks it
//     `option idempotency_level = NO_SIDE_EFFECTS` or `IDEMPOTENT`.
//   - nonRetryableMethods: the RPC changes state (creates, updates, deletes,
//     sends, publishes events or issues tokens). Writes that are idempotent by
//     design are listed here too unless the proto says so, so the decision
//     stays visible in the contract.

// retryableMethods are read-only RPCs that are safe to call again.
var retryableMethods = map[string]struct{}{
	"/swiftmed.analytics.v1.AnalyticsService/GetPharmacyStats": {},
	"/swiftmed.analytics.v1.AnalyticsService/GetUnmetDemand":   {},

	"/swiftmed.auth.v1.AuthService/ValidateToken": {},

	"/swiftmed.catalog.v1.CatalogService/BatchGetProducts":       {},
	"/swiftmed.catalog.v1.CatalogService/GetProduct":             {},
	"/swiftmed.catalog.v1.CatalogService/ListProductsByMolecule": {},
	"/swiftmed.catalog.v1.CatalogService/ResolveName":            {},

	"/swiftmed.expiry.v1.ExpiryService/GetHoldDuration": {},

	"/swiftmed.feedback.v1.FeedbackService/ListPharmacyFeedback": {},

	"/swiftmed.freshness.v1.FreshnessService/GetFreshness": {},

	"/swiftmed.geo.v1.GeoService/FindNearbyPharmacies": {},

	"/swiftmed.holds.v1.HoldsService/GetHold":   {},
	"/swiftmed.holds.v1.HoldsService/ListHolds": {},

	"/swiftmed.inventory.v1.InventoryService/BatchGetStock":     {},
	"/swiftmed.inventory.v1.InventoryService/GetStock":          {},
	"/swiftmed.inventory.v1.InventoryService/ListPharmacyStock": {},

	"/swiftmed.notification.v1.NotificationService/ListNotifications": {},

	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/BatchGetPharmacies": {},
	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/GetPharmacy":        {},
	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/ListPharmacies":     {},

	"/swiftmed.pricing.v1.PricingService/BatchGetPrices": {},

	"/swiftmed.ranking.v1.RankingService/RankOffers": {},

	"/swiftmed.search.v1.SearchService/Autocomplete":   {},
	"/swiftmed.search.v1.SearchService/SearchMedicine": {},

	"/swiftmed.stocksync.v1.StockSyncService/GetSyncJob":   {},
	"/swiftmed.stocksync.v1.StockSyncService/ListSyncJobs": {},

	"/swiftmed.substitution.v1.SubstitutionService/GetSubstitutes": {},

	"/swiftmed.trust.v1.TrustService/BatchGetTrust": {},
	"/swiftmed.trust.v1.TrustService/GetTrust":      {},

	"/swiftmed.userprofile.v1.UserProfileService/GetProfile": {},

	"/swiftmed.watchlist.v1.WatchlistService/ListSubscriptions": {},
	// Server streaming, so the unary interceptor never sees it; listed for completeness.
	"/swiftmed.watchlist.v1.WatchlistService/StreamAlerts": {},
}

// nonRetryableMethods change state and are called at most once per client call.
var nonRetryableMethods = map[string]struct{}{
	"/swiftmed.auth.v1.AuthService/Login":    {}, // issues a refresh token
	"/swiftmed.auth.v1.AuthService/Logout":   {},
	"/swiftmed.auth.v1.AuthService/Register": {},
	// Rotates the refresh token: a retry would present a token already used.
	"/swiftmed.auth.v1.AuthService/RefreshToken": {},
	// Makes the account a pharmacy's staff account (SCRUM-141). Idempotent, but
	// it changes state, so the gateway calls it once and reports any failure.
	"/swiftmed.auth.v1.AuthService/LinkPharmacy": {},

	"/swiftmed.catalog.v1.CatalogService/CreateProduct": {},

	// Documented as idempotent, but it expires holds and calls ExpireHold.
	"/swiftmed.expiry.v1.ExpiryService/RunSweep": {},

	"/swiftmed.feedback.v1.FeedbackService/SubmitFeedback": {}, // publishes FeedbackSubmitted

	// Documented as idempotent, but it rewrites stored scores.
	"/swiftmed.freshness.v1.FreshnessService/ApplyDecay":         {},
	"/swiftmed.freshness.v1.FreshnessService/RecordConfirmation": {},

	"/swiftmed.geo.v1.GeoService/RemovePharmacyLocation": {},
	"/swiftmed.geo.v1.GeoService/UpsertPharmacyLocation": {},

	"/swiftmed.holds.v1.HoldsService/CancelHold":  {},
	"/swiftmed.holds.v1.HoldsService/CollectHold": {},
	"/swiftmed.holds.v1.HoldsService/CreateHold":  {}, // publishes HoldRequested
	"/swiftmed.holds.v1.HoldsService/ExpireHold":  {},

	"/swiftmed.inventory.v1.InventoryService/BulkSetStock":       {}, // may publish StockIncreased
	"/swiftmed.inventory.v1.InventoryService/CommitReservation":  {},
	"/swiftmed.inventory.v1.InventoryService/ReleaseReservation": {},
	"/swiftmed.inventory.v1.InventoryService/SetStock":           {}, // may publish StockIncreased

	"/swiftmed.notification.v1.NotificationService/SendNotification": {},

	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/RegisterPharmacy":      {},
	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/SetVerificationStatus": {},
	"/swiftmed.pharmacyregistry.v1.PharmacyRegistryService/UpdatePharmacy":        {}, // changes pharmacy details and may resubmit it for review

	"/swiftmed.pricing.v1.PricingService/SetPrices": {},

	"/swiftmed.stocksync.v1.StockSyncService/UploadStockCsv": {},

	"/swiftmed.userprofile.v1.UserProfileService/UpdateProfile": {},

	"/swiftmed.watchlist.v1.WatchlistService/Subscribe":   {},
	"/swiftmed.watchlist.v1.WatchlistService/Unsubscribe": {},
}

// isIdempotentMethod reports whether method (a full gRPC method name such as
// "/swiftmed.holds.v1.HoldsService/GetHold") may be retried. Unknown methods
// may not: an RPC is retried only after someone has decided it is safe.
func isIdempotentMethod(method string) bool {
	_, ok := retryableMethods[method]
	return ok
}
