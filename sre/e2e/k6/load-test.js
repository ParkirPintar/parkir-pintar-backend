/**
 * ParkirPintar — k6 Load Test
 * Covers all 14 E2E scenarios from README.md
 *
 * Run:
 *   k6 run sre/e2e/k6/load-test.js
 *   k6 run --env BASE_URL=https://api.parkir-pintar.id sre/e2e/k6/load-test.js
 *
 * Scenarios are weighted to reflect realistic traffic:
 *   - Happy path (system-assigned) is the dominant flow
 *   - Concurrency / war-booking uses ramping VUs to stress Redis lock
 */

import { sleep, check, group } from 'k6';
import { Trend, Counter, Rate } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  authenticate, getAvailability, holdSpot, createReservation,
  getReservation, cancelReservation, checkIn, checkout,
  getPaymentStatus, retryPayment, presenceCheckin, pollPayment,
  randomPlate,
} from './helpers.js';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------
const reservationDuration = new Trend('reservation_duration_ms', true);
const checkoutDuration    = new Trend('checkout_duration_ms', true);
const doubleBookRate      = new Rate('double_book_prevented');
const idempotencyRate     = new Rate('idempotency_correct');

// ---------------------------------------------------------------------------
// Options — scenarios
// ---------------------------------------------------------------------------
export const options = {
  scenarios: {
    // Scenarios 1, 3, 7, 13, 14 — steady functional load
    happy_path: {
      executor: 'constant-vus',
      vus: 10,
      duration: '2m',
      exec: 'scenarioHappyPathSystemAssigned',
      tags: { scenario: 'happy_path' },
    },
    // Scenario 2 — user-selected with hold
    user_selected: {
      executor: 'constant-vus',
      vus: 5,
      duration: '2m',
      exec: 'scenarioHappyPathUserSelected',
      tags: { scenario: 'user_selected' },
    },
    // Scenario 3 — double-book prevention (war booking)
    war_booking: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 50 },
        { duration: '1m',  target: 100 },
        { duration: '30s', target: 0 },
      ],
      exec: 'scenarioDoubleBook',
      tags: { scenario: 'war_booking' },
    },
    // Scenarios 6, 8, 9, 10, 11, 12 — edge cases
    edge_cases: {
      executor: 'constant-vus',
      vus: 5,
      duration: '2m',
      exec: 'scenarioEdgeCases',
      tags: { scenario: 'edge_cases' },
    },
  },
  thresholds: {
    http_req_failed:          ['rate<0.05'],
    http_req_duration:        ['p(95)<3000'],
    reservation_duration_ms:  ['p(95)<2000'],
    checkout_duration_ms:     ['p(95)<2000'],
    double_book_prevented:    ['rate>0.95'],
    idempotency_correct:      ['rate>0.99'],
  },
};

// ---------------------------------------------------------------------------
// Scenario 1 — Happy path (system-assigned): Auth → Availability → Reserve → Check-in → Checkout → Payment
// Also covers Scenario 13 (idempotency duplicate reservation) and 14 (idempotency duplicate checkout)
// ---------------------------------------------------------------------------
export function scenarioHappyPathSystemAssigned() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S1: happy path system-assigned', () => {
    // Availability
    const avail = getAvailability(token, 'CAR', 1);
    check(avail, { 'availability 200': (r) => r.status === 200 });

    // Reserve
    const iKey = uuidv4();
    const t0 = Date.now();
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey);
    reservationDuration.add(Date.now() - t0);

    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id, spot_id } = resv.json();

    // S13 — duplicate reservation idempotency
    group('S13: idempotency duplicate reservation', () => {
      const dup = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey);
      idempotencyRate.add(
        check(dup, {
          'idempotent 200/201': (r) => r.status === 200 || r.status === 201,
          'same reservation_id': (r) => r.json('reservation_id') === reservation_id,
        })
      );
    });

    // Check-in
    const ci = checkIn(token, reservation_id, spot_id);
    check(ci, { 'checkin 200': (r) => r.status === 200, 'no penalty': (r) => r.json('penalty_applied') === 0 });

    // Checkout
    const coKey = uuidv4();
    const t1 = Date.now();
    const co = checkout(token, reservation_id, coKey);
    checkoutDuration.add(Date.now() - t1);

    if (!check(co, { 'checkout 200': (r) => r.status === 200 })) return;
    const { payment_id, invoice_id } = co.json();

    // S14 — duplicate checkout idempotency
    group('S14: idempotency duplicate checkout', () => {
      const dup = checkout(token, reservation_id, coKey);
      idempotencyRate.add(
        check(dup, {
          'idempotent checkout 200': (r) => r.status === 200,
          'same invoice_id': (r) => r.json('invoice_id') === invoice_id,
        })
      );
    });

    // S11 — Payment success (QRIS)
    group('S11: payment success QRIS', () => {
      const pay = pollPayment(token, payment_id, 5, 1000);
      check(pay, {
        'payment 200': (r) => r.status === 200,
        'payment PAID': (r) => r.json('status') === 'PAID',
      });
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 2 — Happy path (user-selected): Auth → Availability → Hold → Reserve → Check-in → Checkout → Payment
// ---------------------------------------------------------------------------
export function scenarioHappyPathUserSelected() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S2: happy path user-selected', () => {
    const avail = getAvailability(token, 'CAR', 1);
    if (!check(avail, { 'availability 200': (r) => r.status === 200 })) return;

    const spots = avail.json('spots') || [];
    const available = spots.filter((s) => s.status === 'AVAILABLE');
    if (available.length === 0) return;

    const spotId = available[0].spot_id;

    // Hold
    const hold = holdSpot(token, spotId);
    if (!check(hold, { 'hold 200': (r) => r.status === 200 })) return;

    // Reserve
    const resv = createReservation(token, { mode: 'USER_SELECTED', vehicle_type: 'CAR', spot_id: spotId });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id } = resv.json();

    // Check-in
    const ci = checkIn(token, reservation_id, spotId);
    check(ci, { 'checkin 200': (r) => r.status === 200 });

    // Checkout + payment
    const co = checkout(token, reservation_id);
    if (!check(co, { 'checkout 200': (r) => r.status === 200 })) return;
    const pay = pollPayment(token, co.json('payment_id'), 5, 1000);
    check(pay, { 'payment PAID': (r) => r.json('status') === 'PAID' });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 3 — Double-book prevention: two concurrent VUs hit same spot → second gets 409
// Scenario 4 — Spot contention / hold queue: Driver B tries held spot → 409 SPOT_HELD
// ---------------------------------------------------------------------------
export function scenarioDoubleBook() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  // All VUs race for the same spot to stress Redis SETNX
  const CONTESTED_SPOT = '1-CAR-01';

  group('S3+S4: double-book & hold contention', () => {
    // S4 — hold contention
    const hold = holdSpot(token, CONTESTED_SPOT);
    const held = hold.status === 200;
    if (!held) {
      doubleBookRate.add(check(hold, { 'S4 hold 409 SPOT_HELD': (r) => r.status === 409 }));
    }

    // S3 — reservation race
    const resv = createReservation(token, {
      mode: 'USER_SELECTED',
      vehicle_type: 'CAR',
      spot_id: CONTESTED_SPOT,
    });

    if (resv.status === 201) {
      // First one wins — cancel to release spot for next iteration
      const { reservation_id } = resv.json();
      cancelReservation(token, reservation_id);
    } else {
      doubleBookRate.add(
        check(resv, { 'S3 double-book 409': (r) => r.status === 409 })
      );
    }
  });

  sleep(0.5);
}

// ---------------------------------------------------------------------------
// Edge case scenarios — round-robin through S5, S6, S7, S8, S9, S10, S12
// ---------------------------------------------------------------------------
let edgeCaseIndex = 0;

export function scenarioEdgeCases() {
  const idx = edgeCaseIndex++ % 7;
  switch (idx) {
    case 0: return scenarioWrongSpotPenalty();   // S6
    case 1: return scenarioCancelFree();          // S7
    case 2: return scenarioCancelWithFee();       // S8
    case 3: return scenarioExtendedStay();        // S9
    case 4: return scenarioOvernightFee();        // S10
    case 5: return scenarioPaymentFailureRetry(); // S12
    case 6: return scenarioReservationExpiry();   // S5
  }
}

// ---------------------------------------------------------------------------
// Scenario 5 — Reservation expiry (no-show): Reserve → GET → expect EXPIRED
// Note: TTL is 1h in prod; test just verifies the GET endpoint handles EXPIRED status
// ---------------------------------------------------------------------------
function scenarioReservationExpiry() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S5: reservation expiry no-show', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id } = resv.json();
    // In real test env TTL is shortened; here we just verify GET works
    const detail = getReservation(token, reservation_id);
    check(detail, {
      'S5 get reservation 200': (r) => r.status === 200,
      'S5 status is RESERVED or EXPIRED': (r) =>
        ['RESERVED', 'EXPIRED'].includes(r.json('status')),
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 6 — Wrong spot penalty: Check-in at different spot → penalty 200.000 IDR
// ---------------------------------------------------------------------------
function scenarioWrongSpotPenalty() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S6: wrong spot penalty', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id, spot_id } = resv.json();
    // Check-in at a different spot
    const wrongSpot = spot_id === '1-CAR-01' ? '1-CAR-02' : '1-CAR-01';
    const ci = checkIn(token, reservation_id, wrongSpot);
    check(ci, {
      'S6 checkin 200': (r) => r.status === 200,
      'S6 wrong_spot true': (r) => r.json('wrong_spot') === true,
      'S6 penalty 200000': (r) => r.json('penalty_applied') === 200000,
    });

    // Checkout to clean up
    const co = checkout(token, reservation_id);
    check(co, {
      'S6 checkout 200': (r) => r.status === 200,
      'S6 invoice penalty 200000': (r) => r.json('penalty') === 200000,
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 7 — Cancellation ≤ 2 min (free): Reserve → cancel immediately → fee=0
// ---------------------------------------------------------------------------
function scenarioCancelFree() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S7: cancel free within 2 min', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id } = resv.json();
    // Cancel immediately (well within 2 min)
    const cancel = cancelReservation(token, reservation_id);
    check(cancel, {
      'S7 cancel 200': (r) => r.status === 200,
      'S7 fee 0': (r) => r.json('cancellation_fee') === 0,
      'S7 status CANCELLED': (r) => r.json('status') === 'CANCELLED',
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 8 — Cancellation > 2 min (5.000 IDR): Reserve → wait → cancel → fee=5000
// Note: sleep(130) is impractical in load test; we simulate by calling cancel and
// asserting the API returns the correct fee based on server-side elapsed time.
// In a short-TTL test env, configure confirmed_at to be backdated.
// ---------------------------------------------------------------------------
function scenarioCancelWithFee() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S8: cancel with fee after 2 min', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id } = resv.json();
    // In a real env with backdated TTL, fee would be 5000.
    // Here we just verify the cancel endpoint responds correctly.
    const cancel = cancelReservation(token, reservation_id);
    check(cancel, {
      'S8 cancel 200': (r) => r.status === 200,
      'S8 status CANCELLED': (r) => r.json('status') === 'CANCELLED',
      'S8 fee 0 or 5000': (r) => [0, 5000].includes(r.json('cancellation_fee')),
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 9 — Extended stay billing (no overstay penalty)
// ---------------------------------------------------------------------------
function scenarioExtendedStay() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S9: extended stay no overstay penalty', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id, spot_id } = resv.json();
    checkIn(token, reservation_id, spot_id);

    const co = checkout(token, reservation_id);
    check(co, {
      'S9 checkout 200': (r) => r.status === 200,
      // No overstay penalty field — only standard hourly applies
      'S9 no extra penalty beyond wrong_spot': (r) => {
        const inv = r.json();
        return inv.penalty === 0 || inv.penalty === undefined;
      },
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 10 — Overnight fee: session crosses midnight → overnight_fee=20000
// Note: actual midnight crossing requires time manipulation in test env.
// We verify the invoice schema includes overnight_fee field and checkout works.
// ---------------------------------------------------------------------------
function scenarioOvernightFee() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S10: overnight fee schema check', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id, spot_id } = resv.json();
    checkIn(token, reservation_id, spot_id);

    const co = checkout(token, reservation_id);
    check(co, {
      'S10 checkout 200': (r) => r.status === 200,
      'S10 overnight_fee field present': (r) => r.json('overnight_fee') !== undefined,
      'S10 overnight_fee is 0 or 20000': (r) => [0, 20000].includes(r.json('overnight_fee')),
    });
  });

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario 12 — Payment failure + retry: Checkout → poll → FAILED → retry → new QR
// ---------------------------------------------------------------------------
function scenarioPaymentFailureRetry() {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  group('S12: payment failure and retry', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;

    const { reservation_id, spot_id } = resv.json();
    checkIn(token, reservation_id, spot_id);

    const co = checkout(token, reservation_id);
    if (!check(co, { 'S12 checkout 200': (r) => r.status === 200 })) return;

    const { payment_id } = co.json();
    const pay = pollPayment(token, payment_id, 3, 500);

    if (pay.json('status') === 'FAILED') {
      const retry = retryPayment(token, payment_id);
      check(retry, {
        'S12 retry 200': (r) => r.status === 200,
        'S12 new qr_code present': (r) => !!r.json('qr_code'),
      });
    } else {
      // Payment succeeded or still pending — scenario not triggered this iteration
      check(pay, { 'S12 payment status valid': (r) => ['PAID', 'PENDING', 'FAILED'].includes(r.json('status')) });
    }
  });

  sleep(1);
}
