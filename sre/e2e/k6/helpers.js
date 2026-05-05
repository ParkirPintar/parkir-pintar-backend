/**
 * ParkirPintar — k6 Helper Functions
 *
 * No authentication required — driver_id is passed in request body.
 * Idempotency-Key header used for POST mutations.
 */

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import { sleep } from 'k6';
import http from 'k6/http';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

// Standard JSON headers
export function jsonHeaders(idempotencyKey) {
  var headers = { 'Content-Type': 'application/json' };
  if (idempotencyKey) {
    headers['Idempotency-Key'] = idempotencyKey;
  }
  return headers;
}

// Generate a random driver_id UUID per VU
export function generateDriverId() {
  return uuidv4();
}

// --- Search ---

export function getAvailability(vehicleType, floor) {
  var params = 'vehicle_type=' + vehicleType;
  if (floor) {
    params = params + '&floor=' + floor;
  }
  return http.get(BASE_URL + '/v1/availability?' + params, {
    headers: { 'Content-Type': 'application/json' },
  });
}

export function getFirstAvailable(vehicleType) {
  return http.get(BASE_URL + '/v1/availability/first?vehicle_type=' + vehicleType, {
    headers: { 'Content-Type': 'application/json' },
  });
}

// --- Reservation ---

export function holdSpot(spotId, driverId) {
  return http.post(
    BASE_URL + '/v1/spots/' + spotId + '/hold',
    JSON.stringify({ driver_id: driverId }),
    { headers: jsonHeaders() }
  );
}

export function createReservation(driverId, body, idempotencyKey) {
  var payload = {
    driver_id: driverId,
    mode: body.mode,
    vehicle_type: body.vehicle_type,
  };
  if (body.spot_id) {
    payload.spot_id = body.spot_id;
  }
  return http.post(
    BASE_URL + '/v1/reservations',
    JSON.stringify(payload),
    { headers: jsonHeaders(idempotencyKey || uuidv4()) }
  );
}

export function getReservation(reservationId) {
  return http.get(BASE_URL + '/v1/reservations/' + reservationId, {
    headers: { 'Content-Type': 'application/json' },
  });
}

export function cancelReservation(reservationId) {
  return http.del(BASE_URL + '/v1/reservations/' + reservationId, null, {
    headers: { 'Content-Type': 'application/json' },
  });
}

// --- Presence / Check-in ---

export function checkIn(reservationId, spotId) {
  return http.post(
    BASE_URL + '/v1/checkin',
    JSON.stringify({ reservation_id: reservationId, spot_id: spotId }),
    { headers: jsonHeaders() }
  );
}

export function updateLocation(reservationId, lat, lng) {
  return http.post(
    BASE_URL + '/v1/presence/location',
    JSON.stringify({
      reservation_id: reservationId,
      latitude: lat,
      longitude: lng,
    }),
    { headers: jsonHeaders() }
  );
}

// --- Billing ---

export function checkout(reservationId, idempotencyKey) {
  return http.post(
    BASE_URL + '/v1/checkout',
    JSON.stringify({ reservation_id: reservationId }),
    { headers: jsonHeaders(idempotencyKey || uuidv4()) }
  );
}

// --- Presence / Exit Gate ---

export function checkOutGate(reservationId) {
  return http.post(
    BASE_URL + '/v1/checkout/gate',
    JSON.stringify({ reservation_id: reservationId }),
    { headers: jsonHeaders() }
  );
}

// --- Payment ---

export function getPaymentStatus(paymentId) {
  return http.get(BASE_URL + '/v1/payments/' + paymentId, {
    headers: { 'Content-Type': 'application/json' },
  });
}

export function retryPayment(paymentId, idempotencyKey) {
  return http.post(
    BASE_URL + '/v1/payments/' + paymentId + '/retry',
    null,
    { headers: jsonHeaders(idempotencyKey || uuidv4()) }
  );
}

// Poll payment until non-PENDING or maxAttempts reached
export function pollPayment(paymentId, maxAttempts, intervalMs) {
  maxAttempts = maxAttempts || 5;
  intervalMs = intervalMs || 1000;
  var res;
  for (var i = 0; i < maxAttempts; i++) {
    res = getPaymentStatus(paymentId);
    if (res.status === 200) {
      var status = res.json('status');
      if (status !== 'PENDING') return res;
    }
    if (i < maxAttempts - 1) sleep(intervalMs / 1000);
  }
  return getPaymentStatus(paymentId);
}

// Poll reservation until reservation_id is resolved (async queue processing)
// The system processes reservations async via RabbitMQ — first response may have empty reservation_id.
// Re-sending with same idempotency key returns the resolved reservation once processed.
// 503 means spot temporarily held — retry is appropriate.
export function pollReservation(driverId, body, idempotencyKey, maxAttempts) {
  maxAttempts = maxAttempts || 10;
  if (!idempotencyKey) idempotencyKey = uuidv4();
  var res;
  for (var i = 0; i < maxAttempts; i++) {
    res = createReservation(driverId, body, idempotencyKey);
    if (res.status === 200 || res.status === 201) {
      var resId = res.json('reservation_id');
      if (resId && resId !== '') return res;
    } else if (res.status === 503) {
      // Spot temporarily held — retry with backoff
      sleep(0.5);
      continue;
    } else {
      // Non-retryable error (409 spot taken, 400 bad request, etc.)
      return res;
    }
    sleep(0.3);
  }
  return res;
}
