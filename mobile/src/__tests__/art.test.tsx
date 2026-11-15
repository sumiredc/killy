import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { render, screen } from '@testing-library/react-native';
import type { ReactTestRendererJSON } from 'react-test-renderer';

import { HoodIcon } from '../art/HoodIcon';
import { Killy } from '../art/Killy';
import { Necromancer } from '../art/Necromancer';
import { Soul } from '../art/Soul';
import { Zombie } from '../art/Zombie';

// The artwork is ported from mock/art.js without redrawing (DESIGN: Characters). Each test draws
// a component and compares its shapes, in order, with the shapes the mock's function returns.

type Shape = { kind: string; geometry: Record<string, string>; fill?: string; stroke?: string };

const geometry = ['d', 'cx', 'cy', 'r', 'rx', 'ry', 'x', 'y', 'width', 'height'];

function mockSVG(fn: 'killySVG' | 'parentSVG' | 'zombieSVG' | 'soulSVG' | 'hoodIconSVG'): string {
  const src = readFileSync(join(__dirname, '../../../mock/art.js'), 'utf8');
  return new Function(`${src}\nreturn ${fn}();`)() as string;
}

/** `#000` becomes `#000000` and `#FFF` `#ffffff`; `url(#x)` stays as it is. */
function colour(v: string | undefined): string | undefined {
  if (v === undefined || v === 'none' || v === 'currentColor') {
    return undefined;
  }
  if (!v.startsWith('#')) {
    return v;
  }
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(v);
  return (short ? `#${short[1]}${short[1]}${short[2]}${short[2]}${short[3]}${short[3]}` : v).toLowerCase();
}

function mockShapes(svg: string): Shape[] {
  const shapes: Shape[] = [];
  for (const m of svg.matchAll(/<(path|ellipse|circle|rect)\b([^>]*?)\/?>/g)) {
    const attrs = Object.fromEntries([...m[2].matchAll(/([\w-]+)="([^"]*)"/g)].map((a) => [a[1], a[2]]));
    shapes.push({
      kind: m[1],
      geometry: Object.fromEntries(geometry.filter((g) => attrs[g] !== undefined).map((g) => [g, attrs[g]])),
      // SVG fills a shape black when it names no fill.
      fill: colour(attrs.fill ?? '#000'),
      stroke: colour(attrs.stroke),
    });
  }
  return shapes;
}

const hostKinds: Record<string, string> = { RNSVGPath: 'path', RNSVGEllipse: 'ellipse', RNSVGCircle: 'circle', RNSVGRect: 'rect' };

/** react-native-svg passes a solid colour as an ARGB number and a gradient by its id. */
function brush(b: unknown): string | undefined {
  if (b === null || typeof b !== 'object') {
    return undefined;
  }
  const v = b as { type: number; payload?: number; brushRef?: string };
  if (v.type === 1) {
    return `url(#${v.brushRef})`;
  }
  return `#${((v.payload ?? 0) >>> 0).toString(16).padStart(8, '0').slice(2)}`;
}

function drawnShapes(): Shape[] {
  const shapes: Shape[] = [];
  const walk = (n: ReactTestRendererJSON | string | null) => {
    if (n === null || typeof n === 'string') {
      return;
    }
    const kind = hostKinds[n.type];
    if (kind) {
      const p = n.props as Record<string, unknown>;
      shapes.push({
        kind,
        geometry: Object.fromEntries(geometry.filter((g) => p[g] !== undefined).map((g) => [g, String(p[g])])),
        fill: brush(p.fill),
        stroke: brush(p.stroke),
      });
    }
    (n.children ?? []).forEach(walk);
  };
  walk(screen.toJSON() as ReactTestRendererJSON);
  return shapes;
}

test('Killy has exactly the shapes of the mock, so nothing was added to it, a mouth least of all', async () => {
  await render(<Killy width={164} />);
  const want = mockShapes(mockSVG('killySVG'));
  expect(want.length).toBeGreaterThan(10);
  expect(drawnShapes()).toEqual(want);
});

test('the necromancer has exactly the shapes of the mock', async () => {
  await render(<Necromancer width={150} />);
  expect(drawnShapes()).toEqual(mockShapes(mockSVG('parentSVG')));
});

test.each([
  [false, 'face-dead'],
  [true, 'face-normal'],
])('a zombie with dead=%s has the shapes of the mock without its %s', async (dead, hidden) => {
  await render(<Zombie width={48} dead={dead} />);
  const svg = mockSVG('zombieSVG').replace(new RegExp(`<g class="${hidden}">[\\s\\S]*?</g>`), '');
  expect(drawnShapes()).toEqual(mockShapes(svg));
});

test('the soul has exactly the shapes of the mock', async () => {
  await render(<Soul width={40} />);
  expect(drawnShapes()).toEqual(mockShapes(mockSVG('soulSVG')));
});

test('the hood icon has the shapes of the mock; its colours come from the counter', async () => {
  await render(<HoodIcon current />);
  const strip = (s: Shape[]) => s.map(({ kind, geometry: g }) => ({ kind, geometry: g }));
  expect(strip(drawnShapes())).toEqual(strip(mockShapes(mockSVG('hoodIconSVG'))));
});
