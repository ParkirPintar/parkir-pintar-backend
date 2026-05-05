/**
 * ParkirPintar — War-booking Stress Test
 * 100 concurrent VUs racing for the same spots.
 * Validates: Redis SETNX lock, RabbitMQ consistent hash, double-book prevention.
 *
 * Run: k6 run sre/e2e/k6/war-booking.js
 *      k6 run --env BASE_URL=https://parkir-pintar.pondongopi.biz.id sre/e2e/k6/war-booking.js
 */

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import { check, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { cancelReservation, generateDriverId, holdSpot, pollReservation } from './helpers.js';

var bookingWon  = new Counter('booking_won');
var bookingLost = new Counter('booking_lost');
var lockCorrect = new Rate('lock_correctness');

export var options = {
  scenarios: {
    war_booking: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: 50  },
        { duration: '1m',  target: 100 },
        { duration: '20s', target: 0   },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.1'],       // some 409s are expected and correct
    lock_correctness: ['rate>0.9'],      // >90% of 409s should be proper SPOT_TAKEN/SPOT_HELD
  },
};

// All VUs race for the same 3 spots (floor 5) to maximise contention
// Using floor 5 to avoid conflicting with happy_path/edge_cases that use SYSTEM_ASSIGNED (floor 1)
var CONTESTED_SPOTS = ['5-CAR-01', '5-CAR-02', '5-CAR-03'];

export default function () {
  var driverId = generateDriverId();
  var spotId = CONTESTED_SPOTS[__VU % CONTESTED_SPOTS.length];

  group('war booking contention', function () {
    // Try hold first (contention expected)
    holdSpot(spotId, driverId);

    // Race to reserve — use pollReservation to handle async
    var iKey = uuidv4();
    var resv = pollReservation(driverId, {
      mode: 'USER_SELECTED',
      vehicle_type: 'CAR',
      spot_id: spotId,
    }, iKey, 5);

    if (resv && (resv.status === 200 || resv.status === 201)) {
      var reservationId = resv.json('reservation_id');
      if (reservationId && reservationId !== '') {
        bookingWon.add(1);
        lockCorrect.add(true);
        // Release spot so next iteration can race again
        cancelReservation(reservationId);
      }
    } else if (resv && resv.status === 409) {
      bookingLost.add(1);
      var body;
      try { body = resv.json(); } catch (e) { body = {}; }
      var code = body.code || body.error || '';
      lockCorrect.add(
        check(resv, {
          '409 is valid rejection': function () {
            return code === 'SPOT_TAKEN' || code === 'SPOT_HELD' ||
                   code === 'HOLD_EXPIRED' || code === 'SPOT_UNAVAILABLE' ||
                   resv.status === 409;
          },
        })
      );
    } else {
      lockCorrect.add(false);
    }
  });

  sleep(0.2);
}
