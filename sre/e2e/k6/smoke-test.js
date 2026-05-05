/**
 * ParkirPintar — Smoke Test
 * 1 VU, 1 iteration — quick validation that all endpoints respond.
 *
 * Run: k6 run sre/e2e/k6/smoke-test.js
 *      k6 run --env BASE_URL=https://parkir-pintar.pondongopi.biz.id sre/e2e/k6/smoke-test.js
 */

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import { check, group, sleep } from 'k6';
import {
    cancelReservation, checkIn,
    checkout, checkOutGate,
    createReservation,
    generateDriverId, getAvailability, getFirstAvailable, holdSpot,
    pollPayment,
    pollReservation,
    retryPayment,
    updateLocation
} from './helpers.js';

export var options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(99)<5000'],
  },
};

export default function () {
  var driverId = generateDriverId();

  // S1 — system-assigned happy path
  group('S1: system-assigned', function () {
    var avail = getAvailability('CAR');
    check(avail, { 'availability 200': function (r) { return r.status === 200; } });

    var first = getFirstAvailable('CAR');
    check(first, { 'first available 200': function (r) { return r.status === 200; } });

    var iKey = uuidv4();
    var resv = pollReservation(driverId, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey, 10);
    if (!check(resv, { 'reserve 200/201': function (r) { return r.status === 200 || r.status === 201; } })) return;
    var reservationId = resv.json('reservation_id');
    var spotId = resv.json('spot_id');
    if (!reservationId) { console.error('reservation_id not resolved'); return; }

    // Idempotency check
    var dup = createReservation(driverId, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey);
    check(dup, { 'idempotent same id': function (r) { return r.json('reservation_id') === reservationId; } });

    // Check-in
    var ci = checkIn(reservationId, spotId);
    check(ci, {
      'checkin 200': function (r) { return r.status === 200; },
      'status ACTIVE': function (r) { return r.json('status') === 'ACTIVE'; },
      'no wrong_spot': function (r) { return r.json('wrong_spot') === false; },
    });

    // Location update
    var loc = updateLocation(reservationId, -6.2, 106.816);
    check(loc, { 'location 200': function (r) { return r.status === 200; } });

    // Checkout
    var coKey = uuidv4();
    var co = checkout(reservationId, coKey);
    if (!check(co, { 'checkout 200': function (r) { return r.status === 200; } })) return;

    // Checkout idempotency
    var dupCo = checkout(reservationId, coKey);
    check(dupCo, { 'idempotent same invoice': function (r) { return r.json('invoice_id') === co.json('invoice_id'); } });

    // Payment
    var paymentId = co.json('payment_id');
    if (paymentId) {
      var pay = pollPayment(paymentId, 5, 1000);
      check(pay, { 'payment status valid': function (r) {
        var s = r.json('status');
        return s === 'PAID' || s === 'PENDING' || s === 'FAILED';
      }});
    }

    // Exit gate
    var gate = checkOutGate(reservationId);
    check(gate, { 'exit gate 200': function (r) { return r.status === 200; } });
  });

  sleep(1);

  // S2 — user-selected
  group('S2: user-selected', function () {
    var driverId2 = generateDriverId();
    var avail = getAvailability('CAR', 1);
    var spots = (avail.json('spots') || []);
    var available = [];
    for (var i = 0; i < spots.length; i++) {
      if (spots[i].status === 'AVAILABLE') available.push(spots[i]);
    }
    if (!available.length) { console.log('no available spots for user-selected'); return; }

    var spotId = available[0].spot_id;
    var hold = holdSpot(spotId, driverId2);
    check(hold, { 'hold 200': function (r) { return r.status === 200; } });

    var iKey = uuidv4();
    var resv = pollReservation(driverId2, { mode: 'USER_SELECTED', vehicle_type: 'CAR', spot_id: spotId }, iKey, 10);
    if (!check(resv, { 'reserve 200/201': function (r) { return r.status === 200 || r.status === 201; } })) return;
    var reservationId = resv.json('reservation_id');
    if (!reservationId) return;

    checkIn(reservationId, spotId);
    var co = checkout(reservationId);
    check(co, { 'checkout 200': function (r) { return r.status === 200; } });
    checkOutGate(reservationId);
  });

  sleep(1);

  // S6 — wrong spot
  group('S6: wrong spot', function () {
    var driverId3 = generateDriverId();
    var iKey = uuidv4();
    var resv = pollReservation(driverId3, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey, 10);
    if (!check(resv, { 'reserve 200/201': function (r) { return r.status === 200 || r.status === 201; } })) return;
    var reservationId = resv.json('reservation_id');
    var spotId = resv.json('spot_id');
    if (!reservationId) return;

    var wrongSpot = spotId === '1-CAR-01' ? '1-CAR-02' : '1-CAR-01';
    var ci = checkIn(reservationId, wrongSpot);
    check(ci, {
      'wrong_spot true': function (r) { return r.json('wrong_spot') === true; },
    });
    cancelReservation(reservationId);
  });

  sleep(1);

  // S7 — cancel free
  group('S7: cancel free', function () {
    var driverId4 = generateDriverId();
    var iKey = uuidv4();
    var resv = pollReservation(driverId4, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey, 10);
    if (!check(resv, { 'reserve 200/201': function (r) { return r.status === 200 || r.status === 201; } })) return;
    var reservationId = resv.json('reservation_id');
    if (!reservationId) return;

    var cancel = cancelReservation(reservationId);
    check(cancel, {
      'cancel 200': function (r) { return r.status === 200; },
      'fee 0': function (r) { return r.json('cancellation_fee') === 0; },
      'status CANCELLED': function (r) { return r.json('status') === 'CANCELLED'; },
    });
  });

  sleep(1);

  // S12 — payment failure retry (best-effort)
  group('S12: payment retry', function () {
    var driverId5 = generateDriverId();
    var iKey = uuidv4();
    var resv = pollReservation(driverId5, { mode: 'SYSTEM_ASSIGNED', vehicle_type: 'CAR' }, iKey, 10);
    if (!check(resv, { 'reserve 200/201': function (r) { return r.status === 200 || r.status === 201; } })) return;
    var reservationId = resv.json('reservation_id');
    var spotId = resv.json('spot_id');
    if (!reservationId) return;

    checkIn(reservationId, spotId);
    var co = checkout(reservationId);
    if (!check(co, { 'checkout 200': function (r) { return r.status === 200; } })) return;

    var paymentId = co.json('payment_id');
    if (!paymentId) return;

    var pay = pollPayment(paymentId, 3, 500);
    if (pay && pay.json('status') === 'FAILED') {
      var retry = retryPayment(paymentId);
      check(retry, { 'retry 200': function (r) { return r.status === 200; }, 'new qr_code': function (r) { return !!r.json('qr_code'); } });
    }
  });
}
