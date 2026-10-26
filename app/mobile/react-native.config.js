// Native capabilities are deliberately limited to this workspace's features.
// Unused Taro peer packages must not add camera/location/media permissions.
const disabled = ['expo', 'expo-file-system', '@bam.tech/react-native-image-resizer', '@react-native-camera-roll/camera-roll', '@react-native-clipboard/clipboard', '@react-native-community/geolocation', '@react-native-community/netinfo', '@react-native-community/segmented-control', '@react-native-community/slider', '@react-native-picker/picker', 'react-native-maps', 'react-native-webview', 'react-native-pager-view', 'react-native-svg'];
module.exports = {
  project: { android: { sourceDir: './android' } },
  dependencies: Object.fromEntries(disabled.map(name => [name, { platforms: { android: null, ios: null } }])),
};
