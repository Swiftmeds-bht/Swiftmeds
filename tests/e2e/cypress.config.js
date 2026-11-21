// Cypress end-to-end tests for search -> reserve -> collect (SCRUM-72).
// They drive the SwiftMed web app in a browser, as a patient and, at
// /pharmacy-portal, as a pharmacist (one app since SCRUM-139), against the
// Docker Compose stack. Run them with tests/e2e/run.sh (see README.md).
//
// Every setting can be changed with an environment variable, so the same specs
// run against localhost (the default) or any other stack. Cypress 16: public
// settings are read with Cypress.expose(), secrets with cy.env().
const fromEnv = (name, fallback) => process.env[name] || fallback;

module.exports = {
  e2e: {
    baseUrl: fromEnv("E2E_PATIENT_URL", "http://localhost:3000"),
    specPattern: "cypress/e2e/**/*.cy.js",
    supportFile: "cypress/support/e2e.js",
    // The reservation saga answers within a second or two; a cold Next.js page
    // can take longer on the first visit.
    defaultCommandTimeout: 20000,
    pageLoadTimeout: 60000,
    video: false,
    screenshotOnRunFailure: true,
  },
  expose: {
    gatewayUrl: fromEnv("E2E_GATEWAY_URL", "http://localhost:8080"),
    // Seeded by `make seed` (deploy/seed): a verified pharmacy, a medicine,
    // and the pharmacist linked to that pharmacy.
    pharmacyId: "40000000-0000-4000-8000-000000000001",
    pharmacyName: "Demo Pharmacy Thimphu 01",
    productId: "10000000-0000-4000-8000-000000000001",
    productName: "Demo Paracetamol 500",
    place: "thimphu",
    pharmacistEmail: "demo-tim-001@demo.swiftmeds.test",
    // The expiry test needs the stack started with a short hold time (run.sh does this).
    holdSeconds: Number(fromEnv("E2E_HOLD_SECONDS", "7200")),
  },
  env: {
    pharmacistPassword: fromEnv("E2E_DEMO_PASSWORD", "swiftmeds-demo"),
    // The gateway's WORKER_SHARED_SECRET, to run the expiry sweep.
    workerSecret: fromEnv("E2E_WORKER_SECRET", ""),
  },
};
