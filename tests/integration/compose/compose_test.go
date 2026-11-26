//go:build integration

package compose

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Swiftmeds-bht/Swiftmeds/pkg/messaging"
	eventsv1 "github.com/Swiftmeds-bht/Swiftmeds/proto/gen/go/events/v1"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	pharmacistEmail = "demo-tim-002@demo.swiftmeds.test"
	demoPassword    = "swiftmeds-demo"

	ibuprofenProduct = "10000000-0000-4000-8000-000000000003"
	thimphuPharmacy  = "40000000-0000-4000-8000-000000000002"
	watchProduct     = "10000000-0000-4000-8000-000000000001"
	watchPharmacy    = "40000000-0000-4000-8000-000000000001"
)

func TestCrossServiceIntegration(t *testing.T) {
	apiURL := strings.TrimRight(os.Getenv("GATEWAY_URL"), "/")
	rabbitURL := os.Getenv("RABBITMQ_URL")
	if apiURL == "" || rabbitURL == "" {
		t.Skip("set GATEWAY_URL and RABBITMQ_URL to run against Compose")
	}

	api := &client{base: apiURL, http: &http.Client{Timeout: 15 * time.Second}}
	patient := api.registerPatient(t)
	pharmacist := api.login(t, pharmacistEmail)

	t.Run("search returns seeded inventory", func(t *testing.T) {
		var response struct {
			Results []struct {
				ProductID string `json:"productId"`
				Offers    []struct {
					PharmacyID        string `json:"pharmacyId"`
					QuantityAvailable int32  `json:"quantityAvailable"`
				} `json:"offers"`
			} `json:"results"`
		}
		api.json(t, http.MethodGet,
			"/api/v1/search?q=Ibuprofen%20Tablets&lat=27.4728&lng=89.6393&radius_m=5000",
			"", nil, http.StatusOK, &response)
		if len(response.Results) == 0 || response.Results[0].ProductID != ibuprofenProduct {
			t.Fatalf("results = %+v, want seeded ibuprofen", response.Results)
		}
		if len(response.Results[0].Offers) == 0 ||
			response.Results[0].Offers[0].PharmacyID != thimphuPharmacy ||
			response.Results[0].Offers[0].QuantityAvailable < 1 {
			t.Fatalf("offers = %+v, want available Thimphu stock", response.Results[0].Offers)
		}
	})

	t.Run("reservation saga expires and releases a hold", func(t *testing.T) {
		var created struct {
			HoldID string `json:"holdId"`
		}
		api.json(t, http.MethodPost, "/api/v1/holds", patient, map[string]interface{}{
			"pharmacyId": thimphuPharmacy, "productId": ibuprofenProduct, "quantity": 1,
		}, http.StatusAccepted, &created)
		active := api.waitHold(t, patient, created.HoldID, "HOLD_STATUS_ACTIVE")
		if active["pickupCode"] == "" {
			t.Fatalf("active hold = %+v, want pickup code", active)
		}
		expiresAtValue, ok := active["expiresAt"].(string)
		if !ok || expiresAtValue == "" {
			t.Fatalf("active hold = %+v, want expiry", active)
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, expiresAtValue)
		if err != nil {
			t.Fatalf("parse expiresAt: %v", err)
		}
		var sweep struct {
			Expired []string `json:"expiredHoldIds"`
		}
		for attempt := 0; attempt < 20; attempt++ {
			api.worker(t, "/api/v1/internal/expiry/sweep",
				map[string]string{"asOf": expiresAt.Add(time.Second).Format(time.RFC3339Nano)}, &sweep)
			if contains(sweep.Expired, created.HoldID) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !contains(sweep.Expired, created.HoldID) {
			t.Fatalf("expired hold IDs = %+v, want %s", sweep.Expired, created.HoldID)
		}
		expired := api.waitHold(t, patient, created.HoldID, "HOLD_STATUS_EXPIRED")
		if expired["pickupCode"] != nil && expired["pickupCode"] != "" {
			t.Fatalf("expired hold exposed pickup code: %+v", expired)
		}
	})

	t.Run("watchlist receives one restock alert", func(t *testing.T) {
		var subscription struct {
			ID string `json:"subscriptionId"`
		}
		api.json(t, http.MethodPost, "/api/v1/watchlist", patient, map[string]interface{}{
			"productId": watchProduct,
			"location":  map[string]float64{"latitude": 27.4728, "longitude": 89.6393},
			"radiusM":   5000,
		}, http.StatusCreated, &subscription)
		if subscription.ID == "" {
			t.Fatalf("subscription = %+v", subscription)
		}

		alerts := make(chan map[string]interface{}, 1)
		go api.alert(t, patient, alerts)
		publishStockIncreased(t, rabbitURL, watchPharmacy, watchProduct, 0, 20)
		select {
		case alert := <-alerts:
			if alert["subscriptionId"] != subscription.ID || alert["productId"] != watchProduct {
				t.Fatalf("alert = %+v, want subscription/product match", alert)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("timed out waiting for watchlist alert")
		}
	})

	t.Run("feedback updates trust used by search", func(t *testing.T) {
		var created struct {
			HoldID string `json:"holdId"`
		}
		api.json(t, http.MethodPost, "/api/v1/holds", patient, map[string]interface{}{
			"pharmacyId": thimphuPharmacy, "productId": ibuprofenProduct, "quantity": 1,
		}, http.StatusAccepted, &created)
		active := api.waitHold(t, patient, created.HoldID, "HOLD_STATUS_ACTIVE")
		var collected map[string]interface{}
		api.json(t, http.MethodPost, "/api/v1/holds/"+created.HoldID+"/collect", pharmacist,
			map[string]string{"pickupCode": active["pickupCode"].(string)}, http.StatusOK, &collected)
		if collected["status"] != "HOLD_STATUS_COLLECTED" {
			t.Fatalf("collected hold = %+v", collected)
		}
		var feedback map[string]interface{}
		api.json(t, http.MethodPost, "/api/v1/holds/"+created.HoldID+"/feedback", patient,
			map[string]interface{}{"wasInStock": true, "rating": 5}, http.StatusCreated, &feedback)
		if feedback["feedbackId"] == "" {
			t.Fatalf("feedback = %+v", feedback)
		}

		var search struct {
			Results []struct {
				Offers []struct {
					PharmacyID string  `json:"pharmacyId"`
					TrustScore float64 `json:"trustScore"`
				} `json:"offers"`
			} `json:"results"`
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			api.json(t, http.MethodGet,
				"/api/v1/search?q=Ibuprofen%20Tablets&lat=27.4728&lng=89.6393&radius_m=5000",
				"", nil, http.StatusOK, &search)
			for _, result := range search.Results {
				for _, offer := range result.Offers {
					if offer.PharmacyID == thimphuPharmacy && offer.TrustScore > 0.5 {
						return
					}
				}
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("trust score did not increase: %+v", search.Results)
	})
}

type client struct {
	base string
	http *http.Client
}

func (c *client) login(t *testing.T, email string) string {
	t.Helper()
	var response struct {
		Tokens struct {
			AccessToken string `json:"accessToken"`
		} `json:"tokens"`
	}
	c.json(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": email, "password": demoPassword,
	}, http.StatusOK, &response)
	if response.Tokens.AccessToken == "" {
		t.Fatalf("login %s returned no access token", email)
	}
	return response.Tokens.AccessToken
}

func (c *client) registerPatient(t *testing.T) string {
	t.Helper()
	email := "compose-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.test"
	c.json(t, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": email, "password": demoPassword, "role": "ROLE_PATIENT",
	}, http.StatusCreated, nil)
	return c.login(t, email)
}

func (c *client) json(t *testing.T, method, path, token string, body interface{}, want int, result interface{}) {
	t.Helper()
	var requestBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		requestBody = bytes.NewReader(data)
	}
	request, err := http.NewRequest(method, c.base+path, requestBody)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != want {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.StatusCode, want, data)
	}
	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatalf("decode response: %v: %s", err, data)
		}
	}
}

func (c *client) waitHold(t *testing.T, token, id, want string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var hold map[string]interface{}
	for time.Now().Before(deadline) {
		hold = nil
		c.json(t, http.MethodGet, "/api/v1/holds/"+id, token, nil, http.StatusOK, &hold)
		if hold["status"] == want {
			return hold
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("hold %s did not become %s: %+v", id, want, hold)
	return nil
}

func (c *client) worker(t *testing.T, path string, body interface{}, result interface{}) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal worker request: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create worker request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Worker-Secret", os.Getenv("WORKER_SHARED_SECRET"))
	response, err := c.http.Do(request)
	if err != nil {
		t.Fatalf("worker request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	bodyData, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("worker status = %d: %s", response.StatusCode, bodyData)
	}
	if err := json.Unmarshal(bodyData, result); err != nil {
		t.Fatalf("decode worker response: %v", err)
	}
}

func (c *client) alert(t *testing.T, token string, alerts chan<- map[string]interface{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/watchlist/alerts", nil)
	if err != nil {
		t.Errorf("create alert request: %v", err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := c.http.Do(request)
	if err != nil {
		t.Errorf("alert request: %v", err)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Errorf("alert status = %d, want 200", response.StatusCode)
		return
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var alert map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &alert); err != nil {
			t.Errorf("decode alert: %v", err)
			return
		}
		alerts <- alert
		return
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		t.Errorf("read alert stream: %v", err)
	}
}

func publishStockIncreased(t *testing.T, rabbitURL, pharmacyID, productID string, previous, current int32) {
	t.Helper()
	connection, err := amqp.Dial(rabbitURL)
	if err != nil {
		t.Fatalf("connect RabbitMQ: %v", err)
	}
	defer func() { _ = connection.Close() }()
	channel, err := connection.Channel()
	if err != nil {
		t.Fatalf("open RabbitMQ channel: %v", err)
	}
	defer func() { _ = channel.Close() }()
	publisher, err := messaging.NewPublisher(channel)
	if err != nil {
		t.Fatalf("create event publisher: %v", err)
	}
	event := &eventsv1.StockIncreased{
		Metadata: &eventsv1.EventMetadata{
			OccurredAt: timestamppb.Now(),
			Producer:   "compose-integration-test",
		},
		PharmacyId: pharmacyID, ProductId: productID,
		PreviousAvailable: previous, NewAvailable: current,
	}
	if err := publisher.Publish(context.Background(), "stock.increased", event); err != nil {
		t.Fatalf("publish stock.increased: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
