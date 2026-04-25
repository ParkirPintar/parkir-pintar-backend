/**
 * Smoke test — 1 VU, 1 iteration per scenario
 * Run: k6 run sre/e2e/k6/smoke-test.js
 */

import { check, group, sleep } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  authenticate, getAvailability, holdSpot, createReservation,
  getReservation, cancelReservation, checkIn, checkout,
  pollPayment, retryPayment, randomPlate,
} from './helpers.js';

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(99)<5000'],
  },
};

export default function () {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) { console.error('auth failed'); return; }

  // S1 — system-assigned happy path
  group('S1: system-assigned', () => {
    const avail = getAvailability(token, 'CAR');
    check(avail, { 'availability 200': (r) => r.status === 200 });

    const iKey = uuidv4();
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey);
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id, spot_id } = resv.json();

    // S13 idempotency
    const dup = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey);
    check(dup, { 'S13 idempotent same id': (r) => r.json('reservation_id') === reservation_id });

    const ci = checkIn(token, reservation_id, spot_id);
    check(ci, { 'checkin 200': (r) => r.status === 200, 'no penalty': (r) => r.json('penalty_applied') === 0 });

    const coKey = uuidv4();
    const co = checkout(token, reservation_id, coKey);
    if (!check(co, { 'checkout 200': (r) => r.status === 200 })) return;

    // S14 idempotency
    const dupCo = checkout(token, reservation_id, coKey);
    check(dupCo, { 'S14 idempotent same invoice': (r) => r.json('invoice_id') === co.json('invoice_id') });

    // S11 payment
    const pay = pollPayment(token, co.json('payment_id'), 5, 1000);
    check(pay, { 'S11 payment PAID': (r) => r.json('status') === 'PAID' });
  });

  sleep(1);

  // S2 — user-selected
  group('S2: user-selected', () => {
    const avail = getAvailability(token, 'CAR', 1);
    const spots = (avail.json('spots') || []).filter((s) => s.status === 'AVAILABLE');
    if (!spots.length) return;

    const spotId = spots[0].spot_id;
    const hold = holdSpot(token, spotId);
    check(hold, { 'hold 200': (r) => r.status === 200 });

    const resv = createReservation(token, { mode: 'USER_SELECTED', vehicle_type: 'CAR', spot_id: spotId });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id } = resv.json();

    checkIn(token, reservation_id, spotId);
    const co = checkout(token, reservation_id);
    check(co, { 'checkout 200': (r) => r.status === 200 });
  });

  sleep(1);

  // S6 — wrong spot penalty
  group('S6: wrong spot penalty', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id, spot_id } = resv.json();
    const wrongSpot = spot_id === '1-CAR-01' ? '1-CAR-02' : '1-CAR-01';
    const ci = checkIn(token, reservation_id, wrongSpot);
    check(ci, { 'wrong_spot true': (r) => r.json('wrong_spot') === true, 'penalty 200000': (r) => r.json('penalty_applied') === 200000 });
    checkout(token, reservation_id);
  });

  sleep(1);

  // S7 — cancel free
  group('S7: cancel free', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const cancel = cancelReservation(token, resv.json('reservation_id'));
    check(cancel, { 'cancel 200': (r) => r.status === 200, 'fee 0': (r) => r.json('cancellation_fee') === 0 });
  });

  sleep(1);

  // S12 — payment failure retry (best-effort)
  group('S12: payment retry', () => {
    const resv = createReservation(token, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' });
    if (!check(resv, { 'reserve 201': (r) => r.status === 201 })) return;
    const { reservation_id, spot_id } = resv.json();
    checkIn(token, reservation_id, spot_id);
    const co = checkout(token, reservation_id);
    if (!check(co, { 'checkout 200': (r) => r.status === 200 })) return;
    const pay = pollPayment(token, co.json('payment_id'), 3, 500);
    if (pay.json('status') === 'FAILED') {
      const retry = retryPayment(token, co.json('payment_id'));
      check(retry, { 'retry 200': (r) => r.status === 200, 'new qr_code': (r) => !!r.json('qr_code') });
    }
  });
}
