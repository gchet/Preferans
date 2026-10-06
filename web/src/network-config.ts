import defaults from '../../config/network.json' with {type:'json'};
declare const __BUILD_NETWORK__: Partial<typeof defaults> & {roomServiceURL?: string};
export const networkConfig = {...defaults, ...(typeof __BUILD_NETWORK__ === 'undefined' ? {} : __BUILD_NETWORK__)};
