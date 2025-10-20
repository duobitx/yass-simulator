#ifndef COMMON_H
#define COMMON_H

#define MAXSAT 200

struct sat_ref_dscr { int sid; float dist; };

struct sat_pos_dscr {
   double x, y, z, lat, lon, alt;
   int nref;
   struct sat_ref_dscr sat_ref[MAXSAT];  
 };
 

struct common  {
    int busy, nsat;
    char utc_dttm[32];
    struct sat_pos_dscr sat[MAXSAT];
};

#endif


 