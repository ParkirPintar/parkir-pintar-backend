import http from 'k6/http';
import { check } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8000';

export function authHeaders(token) {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` };
}

export function authenticate(licensePlate, vehicleType) {
  const res = http.post(
    `${BASE_URL}/v1/auth/token`,
    JSON.stringify({ license_plate: licensePlate, vehicle_type: vehicleType }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  check(res, { 'auth 200': (r) => r.status === 200 });
  return res.json('token');
}

export function getAvailability(token, vehicleType, floor) {
  const params = floor ? `vehicle_type=${vehicleType}&floor=${floor}` : `vehicle_type=${vehicleType}`;
  return http.get(`${BASE_URL}/v1/availability?${params}`, { headers: authHeaders(token) });
}

export function holdSpot(token, spotId) {
  return http.post(`${BASE_URL}/v1/spots/${spotId}/hold`, null, { headers: authHeaders(token) });
}

export function createReservation(token, body, idempotencyKey) {
  const headers = { ...authHeaders(token), 'Idempotency-Key': idempotencyKey || uuidv4() };
  return http.post(`${BASE_URL}/v1/reservations`, JSON.stringify(body), { headers });
}

export function getReservation(token, reservationId) {
  return http.get(`${BASE_URL}/v1/reservations/${reservationId}`, { headers: authHeaders(token) });
}

export function cancelReservation(token, reservationId) {
  return http.del(`${BASE_URL}/v1/reservations/${reservationId}`, null, { headers: authHeaders(token) });
}

export function checkIn(token, reservationId, actualSpotId, idempotencyKey) {
  const headers = { ...authHeaders(token), 'Idempotency-Key': idempotencyKey || uuidv4() };
  return http.post(
    `${BASE_URL}/v1/checkin`,
    JSON.stringify({ reservation_id: reservationId, actual_spot_id: actualSpotId }),
    { headers }
  );
}

export function checkout(token, reservationId, idempotencyKey) {
  const headers = { ...authHeaders(token), 'Idempotency-Key': idempotencyKey || uuidv4() };
  return http.post(
    `${BASE_URL}/v1/checkout`,
    JSON.stringify({ reservation_id: reservationId }),
    { headers }
  );
}

export function getPaymentStatus(token, paymentId) {
  return http.get(`${BASE_URL}/v1/payments/${paymentId}`, { headers: authHeaders(token) });
}

export function retryPayment(token, paymentId, idempotencyKey) {
  const headers = { ...authHeaders(token), 'Idempotency-Key': idempotencyKey || uuidv4() };
  return http.post(`${BASE_URL}/v1/payments/${paymentId}/retry`, null, { headers });
}

export function presenceCheckin(token, reservationId, lat, lng) {
  return http.post(
    `${BASE_URL}/v1/presence/checkin`,
    JSON.stringify({ reservation_id: reservationId, latitude: lat, longitude: lng }),
    { headers: authHeaders(token) }
  );
}

// Poll payment until non-PENDING or maxAttempts reached
export function pollPayment(token, paymentId, maxAttempts = 5, intervalMs = 1000) {
  for (let i = 0; i < maxAttempts; i++) {
    const res = getPaymentStatus(token, paymentId);
    if (res.status === 200) {
      const status = res.json('status');
      if (status !== 'PENDING') return res;
    }
    if (i < maxAttempts - 1) sleep(intervalMs / 1000);
  }
  return getPaymentStatus(token, paymentId);
}

// Random plate generator to avoid conflicts between VUs
export function randomPlate() {
  return `B${Math.floor(1000 + Math.random() * 9000)}K6T`;
}
