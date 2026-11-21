# Smoke test for a running stack (SCRUM-66): search -> reserve -> collect,
# every call through the gateway. Windows version of scripts/smoke-test.sh.
#
#   powershell -ExecutionPolicy Bypass -File scripts/smoke-test.ps1
#
# Uses the demo data and accounts that the seed loads (deploy/seed).
$ErrorActionPreference = "Stop"

$port = if ($env:GATEWAY_HTTP_PORT) { $env:GATEWAY_HTTP_PORT } else { "8080" }
$Gateway = if ($env:GATEWAY) { $env:GATEWAY } else { "http://localhost:$port" }
$Password = "swiftmeds-demo"
$Patient = "patient@demo.swiftmeds.test"
# Demo Pharmacy Thimphu 02 stocks Demo Ibuprofen 400 in deploy/seed/stock.csv.
$Pharmacist = "demo-tim-002@demo.swiftmeds.test"
$PharmacyId = "40000000-0000-4000-8000-000000000002"
$ProductId = "10000000-0000-4000-8000-000000000003"
$Query = "Demo Ibuprofen 400"

function Step($text) { Write-Host "`n==> $text" }
function Fail($text) { Write-Host "FAIL: $text" -ForegroundColor Red; exit 1 }
function Login($email) {
    $body = @{ email = $email; password = $Password } | ConvertTo-Json
    try {
        $r = Invoke-RestMethod -Method Post -Uri "$Gateway/api/v1/auth/login" -ContentType "application/json" -Body $body
        return $r.tokens.accessToken
    } catch { Fail "sign-in failed for $email (run the seed to create the demo accounts): $_" }
}

Step "Gateway health"
try { Invoke-RestMethod -Uri "$Gateway/healthz" | Out-Null } catch { Fail "gateway is not reachable at $Gateway (is the stack running?)" }
Write-Host "ok"

Step "Search for `"$Query`" near Thimphu"
$q = [uri]::EscapeDataString($Query)
$search = Invoke-RestMethod -Uri "$Gateway/api/v1/search?q=$q&lat=27.4728&lng=89.6390&radius=5000"
$offer = $search.results.offers | Where-Object { $_.pharmacyId -eq $PharmacyId } | Select-Object -First 1
if (-not $offer) { Fail "search did not return pharmacy $PharmacyId (was the demo data seeded?)" }
Write-Host "ok: $($offer.pharmacyName) has $($offer.quantityAvailable) in stock"

Step "Sign in as the demo patient and reserve 1 unit"
$patientToken = Login $Patient
$patientHeaders = @{ Authorization = "Bearer $patientToken" }
$body = @{ pharmacyId = $PharmacyId; productId = $ProductId; quantity = 1 } | ConvertTo-Json
$hold = Invoke-RestMethod -Method Post -Uri "$Gateway/api/v1/holds" -Headers $patientHeaders -ContentType "application/json" -Body $body
if (-not $hold.holdId) { Fail "reserve failed" }
Write-Host "ok: hold $($hold.holdId) created"

Step "Wait for the reservation saga to confirm the hold"
$code = $null
for ($i = 0; $i -lt 20; $i++) {
    $current = Invoke-RestMethod -Uri "$Gateway/api/v1/holds/$($hold.holdId)" -Headers $patientHeaders
    if ($current.status -eq "HOLD_STATUS_ACTIVE") { $code = $current.pickupCode; break }
    if ($current.status -eq "HOLD_STATUS_REJECTED") { Fail "hold was rejected (no stock left? run the seed again to reset it)" }
    Start-Sleep -Seconds 1
}
if (-not $code) { Fail "hold did not become ACTIVE within 20 s (last status: $($current.status))" }
Write-Host "ok: ACTIVE, pickup code $code"

Step "Sign in as the pharmacist of that pharmacy and collect with the pickup code"
$pharmacistToken = Login $Pharmacist
$body = @{ pickupCode = $code } | ConvertTo-Json
$collected = Invoke-RestMethod -Method Post -Uri "$Gateway/api/v1/holds/$($hold.holdId)/collect" -Headers @{ Authorization = "Bearer $pharmacistToken" } -ContentType "application/json" -Body $body
if ($collected.status -ne "HOLD_STATUS_COLLECTED") { Fail "collect failed: status $($collected.status)" }
Write-Host "ok: COLLECTED"

Write-Host "`nSmoke test passed: search -> reserve -> collect works through the gateway." -ForegroundColor Green
