import sharp from 'sharp';

await sharp(new URL('../public/social-card.svg', import.meta.url).pathname)
  .png()
  .toFile(new URL('../public/og.png', import.meta.url).pathname);
console.log('Rendered public/og.png (1200 × 630).');
