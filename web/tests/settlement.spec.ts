import {test, expect} from "@playwright/test";
import {wholeWhists} from "../src/settlement";

test("whole whists preserve exact integer scores and zero sum for three and four seats", () => {
  expect(wholeWhists([30,9,-39])).toEqual([10,3,-13]);
  expect(wholeWhists([20,-10,-10])).toEqual([7,-3,-4]);
  expect(wholeWhists([10,10,-10,-10])).toEqual([3,3,-3,-3]);
  // Exact chat example and a four-seat example; ties follow seat order.
  expect(wholeWhists([92,-88,-4])).toEqual([31,-29,-2]);
  expect(wholeWhists([106,-134,-22,50])).toEqual([27,-33,-6,12]);
  for (const n of [3,4]) {
    for (let a = -80; a <= 80; a++) {
      const exact = n === 3 ? [a,17,-a-17] : [a,17,-23,6-a];
      const rounded = wholeWhists(exact);
      expect(rounded.reduce((s,v) => s+v,0)).toBe(0);
      rounded.forEach((v,i) => {
        expect(Number.isInteger(v)).toBeTruthy();
        expect(Math.abs(v-exact[i]/n)).toBeLessThan(1);
      });
    }
  }
});
