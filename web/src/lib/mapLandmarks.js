import { PhCrown, PhDiamond, PhDoorOpen, PhFlask, PhKeyhole, PhMountains, PhSparkle, PhSpiral } from '@phosphor-icons/vue';

const portal = { icon: PhDoorOpen, color: '#28786f', priority: 1 };
const cave = { icon: PhMountains, color: '#705543', priority: 3 };
const relic = { icon: PhFlask, color: '#4c6789', priority: 2 };
const styles = {
  pigking: { icon: PhCrown, color: '#a87924', priority: 0 },
  octopusking: { icon: PhCrown, color: '#a87924', priority: 0 },
  volcano: { icon: PhMountains, color: '#b4482a', priority: 0 },
  volcano_altar: { icon: PhFlask, color: '#b4482a', priority: 0 },
  wormhole: { icon: PhSpiral, color: '#7e549a', priority: 4 },
  bermudatriangle: { icon: PhSpiral, color: '#3b7fa8', priority: 4 },
  multiplayer_portal: portal, multiplayer_portal_moonrock: portal, multiplayer_portal_moonrock_constr: portal, monkeyisland_portal: portal,
  seaworthy: portal, skyworthy: portal,
  cave_entrance: cave, cave_entrance_open: cave, cave_exit: cave, volcano_exit: { icon: PhMountains, color: '#b4482a', priority: 1 },
  ancient_altar: relic, ancient_altar_broken: relic, archive_switch: relic, obsidian_workbench: { icon: PhFlask, color: '#b4482a', priority: 1 },
  atrium_gate: { icon: PhKeyhole, color: '#536a7b', priority: 1 },
  slotmachine: { icon: PhSparkle, color: '#b4872a', priority: 1 },
  sharkittenspawner: { icon: PhMountains, color: '#4c6789', priority: 1 },
  packim_fishbone: { icon: PhSparkle, color: '#28786f', priority: 1 },
  doydoy: { icon: PhSparkle, color: '#4f8a55', priority: 1 },
  woodlegs_cage: { icon: PhKeyhole, color: '#87634c', priority: 1 },
  kraken: { icon: PhSparkle, color: '#5b4a6b', priority: 1 },
  resurrectionstone: { icon: PhDiamond, color: '#77675a', priority: 3 },
  tar_pool: { icon: PhDiamond, color: '#3d4753', priority: 3 },
  buriedtreasure: { icon: PhDiamond, color: '#a87924', priority: 3 },
  mermhouse_fisher: { icon: PhDiamond, color: '#416f60', priority: 3 },
};
const landmark = { icon: PhSparkle, color: '#87634c', priority: 2 };

export function landmarkStyle(kind) { return styles[kind] || landmark; }
export function landmarkCoordinates(point, map) {
  return `${Math.round((point.x - map.width / 2) * 4)}, ${Math.round((point.y - map.height / 2) * 4)}`;
}
