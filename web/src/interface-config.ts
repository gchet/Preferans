import settings from '../../config/interface.json' with {type:'json'};
import {networkConfig} from './network-config';
export const interfaceConfig = {...settings, roomServiceURL: networkConfig.roomServiceURL ?? settings.roomServiceURL};
document.documentElement.style.setProperty('--ready-attention-duration',`${settings.readyAttentionDurationMs}ms`);
for(const [name,value] of Object.entries({
  'mini-card-size': settings.miniCardSizePx,
  'mini-card-font-size': settings.miniCardFontSizePx,
  'mini-card-rank-size': settings.miniCardRankSizePx,
  'card-stair-min': settings.cardStairMinPx,
  'card-stair-max': settings.cardStairMaxPx,
})) document.documentElement.style.setProperty(`--${name}`,`${value}px`);
document.documentElement.style.setProperty('--card-stair-vw',`${settings.cardStairViewportRatio*100}vw`);
document.documentElement.style.setProperty('--own-card-scale',String(settings.ownCardScale));
document.documentElement.style.setProperty('--mini-seat-step',`${settings.miniCardSizePx*26/38}px`);
