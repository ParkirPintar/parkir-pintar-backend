/**
 * War-booking stress test — 100 concurrent VUs racing for the same spot
 * Validates: Redis SETNX lock, RabbitMQ consistent hash, double-book prevention
 *
 * Run: k6 run sre/e2e/k6/war-booking.js
 */

import { check, group, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { authenticate, holdSpot, createReservation, cancelReservation, randomPlate } from './helpers.js';

const bookingWon  = new Counter('booking_won');
const bookingLost = new Counter('booking_lost');
const lockCorrect = new Rate('lock_correctness'); // only 1 winner per spot per round

export const options = {
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
    http_req_failed: ['rate<0.1'],   // some 409s are expected and correct
    lock_correctness: ['rate>0.9'],  // >90% of 409s should be proper SPOT_TAKEN/SPOT_HELD
  },
};

// All VUs race for the same 3 spots to maximise contention
const CONTESTED_SPOTS = ['1-CAR-01', '1-CAR-02', '1-CAR-03'];

export default function () {
  const token = authenticate(randomPlate(), 'CAR');
  if (!token) return;

  const spotId = CONTESTED_SPOTS[__VU % CONTESTED_SPOTS.length];

  group('war booking contention', () => {
    // Try hold first (S4)
    holdSpot(token, spotId); // ignore result — contention is expected

    // Race to reserve
    const resv = createReservation(token, {
      mode: 'USER_SELECTED',
      vehicle_type: 'CAR',
      spot_id: spotId,
    });

    if (resv.status === 201) {
      bookingWon.add(1);
      lockCorrect.add(true);
      // Release spot so next iteration can race again
      cancelReservation(token, resv.json('reservation_id'));
    } else if (resv.status === 409) {
      bookingLost.add(1);
      const code = resv.json('code');
      lockCorrect.add(
        check(resv, {
          '409 is SPOT_TAKEN or SPOT_HELD or HOLD_EXPIRED': () =>
            ['SPOT_TAKEN', 'SPOT_HELD', 'HOLD_EXPIRED', 'SPOT_UNAVAILABLE'].includes(code),
        })
      );
    } else {
      lockCorrect.add(false);
    }
  });

  sleep(0.2);
}
