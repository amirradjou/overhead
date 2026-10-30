[out:json][timeout:60];
(
  way["natural"="coastline"](43.595,-79.425,43.648,-79.345);
  way["aeroway"~"runway|apron|terminal|hangar"](43.615,-79.410,43.635,-79.380);
  way["aeroway"="aerodrome"](43.615,-79.410,43.635,-79.380);
  relation["aeroway"="aerodrome"](43.615,-79.410,43.635,-79.380);
  way["man_made"~"breakwater|pier|groyne"](43.600,-79.425,43.645,-79.350);
  way["building"](around:1500,43.6397,-79.3921);
);
out tags geom;
