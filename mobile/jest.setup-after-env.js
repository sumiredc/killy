// Reanimated runs its web implementation under jest (the resolver in package.json), driven by
// jest's fake timers. setUpTests adds toHaveAnimatedStyle and toHaveAnimatedProps.
require('react-native-reanimated').setUpTests();
