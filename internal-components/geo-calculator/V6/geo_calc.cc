#include <time.h>
#include <cstring>
#include <vector>
#include <jansson.h>
#include <iostream>
#include <fstream>
#include <iomanip>
#include <ctime>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <vector>

#include "CoordGeodetic.h"
#include "SGP4.h"
#include "geocalc_message.pb.h"

#define SHM_NAME "/geo_calc_shared_memory"
struct common *pcom;

// The per-node working arrays (sat[], tmp[]) are sized dynamically from the
// FsNodes count in the input JSON (see parse_json), so there is no
// compile-time satellite cap. MAX_FSNODES is only a sanity bound that guards
// against a corrupt/absurd input file.
#define MAX_FSNODES 1000000
struct sun_pos_dscr { double lat, lon, vx, vy, vz; } sun;
struct sat_pos_dscr { double x, y, z; } *sat = nullptr;



int n_sat=0, r_sat=0,  n_bs=0, cont_flag=1, shm, shm_flag=0, shm_bufsize;
time_t tm_sym, tm_start, tm_stop;
float accel=1.0;
char *shm_buf;

yass::fs::GeoCommon message;

std::vector<libsgp4::Tle>  V_tle;
std::vector<libsgp4::SGP4> V_sgp4;
libsgp4::DateTime sgp4dttm;

const double radiusearthkm = 6378.137;
const double PI = 3.141592653589;
const double DEG2RAD = PI / 180.0;
const double RAD2DEG = 180.0 / PI;
// Minimum antenna elevation for a satellite<->ground-station link. A real
// ground antenna cannot work at the horizon (atmospheric attenuation, terrain
// masking, mechanical limits), so links below this angle are treated as blocked.
const double MIN_GS_ELEVATION_DEG = 10.0;


char (*tmp)[3][70] = nullptr;
struct bs_dscr { char name[32]; double lat,lon,alt; };
std::vector<struct bs_dscr>  V_bs;

static int parse_sat(json_t *sat)
{ json_error_t error;
  size_t index;
  json_t *line;
  std::string str;

  json_t *sname = json_object_get(sat, "Name");
   if( !json_is_string(sname) ) { std::cerr << "  satellite name not defined\n"; return 1; }
   //std::cout << "  satelita " << json_string_value(sname) << "\n";
   str=json_string_value(sname); strcpy(tmp[r_sat][0],str.substr(0,63).c_str());
  json_t *tle = json_object_get(sat, "TLE");
   if( !json_is_array(tle) ) { std::cerr << "  TLE for " << json_string_value(sname) << " is not an array\n"; return 1; }
   if( json_array_size(tle) != 2 ) { std::cerr << "  wrong  TLE for " << json_string_value(sname) << "\n"; return 1; }
  json_array_foreach(tle, index, line) {
    if( !json_is_string(line) ) { std::cerr << "  wrong  TLE for " << json_string_value(sname) << "\n"; return 1;  }
    str=json_string_value(line); if( str.length() > 69 ) { std::cerr << "  too long  TLE lines for " << json_string_value(sname) << "\n"; return 1;  }
    strcpy(tmp[r_sat][index+1],str.c_str());
   }
  r_sat++;
  return 0;
}

static int parse_bs(json_t *sat)
{ json_error_t error;
  struct bs_dscr bs;
  std::string str;

  json_t *bsname = json_object_get(sat, "Name");
   if( !json_is_string(bsname) ) { std::cerr << "  station name not defined\n"; return 1; }
   //std::cout << "  stacja " << json_string_value(bsname) << "\n";
   str=json_string_value(bsname); strcpy(bs.name,str.substr(0,63).c_str());
  json_t *geo = json_object_get(sat, "Geo");
   if( !json_is_object(geo) ) { std::cerr << "  Geo for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *lat = json_object_get(geo, "Lat");
   if( json_is_integer(lat) ) bs.lat = json_integer_value(lat);
    else if( json_is_real(lat) ) bs.lat = json_real_value(lat);
    else { std::cerr << "  Lat for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *lon = json_object_get(geo, "Lng");
   if( json_is_integer(lon) ) bs.lon = json_integer_value(lon);
    else if( json_is_real(lon) ) bs.lon = json_real_value(lon);
    else { std::cerr << "  Long for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *alt = json_object_get(geo, "Alt");
   if( alt && !json_is_null(alt) ) {
     if( json_is_integer(alt) ) bs.alt = json_integer_value(alt);
      else if( json_is_real(alt) ) bs.alt = json_real_value(alt);
    }
   else bs.alt=0.0;

  V_bs.push_back(bs); n_bs++;
  return 0;
}

static int parse_json(char *fname)
{ json_error_t error;
  std::tm tm = {};
  size_t index;
  json_t *sat;
  int n;

  json_t *root = json_load_file(fname, 0, &error);
   if (!root) {  std::cerr << "  " << error.text << "\n"; return 1; }
  json_t *ename = json_object_get(root, "Name");
   if( json_is_string(ename) ) std::cout << "  experiment name:  " << json_string_value(ename) << "\n";
  json_t *estart = json_object_get(root, "StartTime");
   if( !json_is_string(estart) ) { std::cerr << "  StartTime not defined\n"; return 1; }
   std::istringstream ss(json_string_value(estart));
   ss >> std::get_time(&tm, "%Y-%m-%dT%H:%M:%S");
   if (ss.fail()) { std::cerr << "  wrong StartTime literal: " << json_string_value(estart) << "\n"; return 1; }
    //tm.tm_mday != d || tm.tm_mon+1 != m || tm.tm_year+1900 != y
   sgp4dttm = libsgp4::DateTime(tm.tm_year+1900,tm.tm_mon+1,tm.tm_mday);
   tm_start=timegm(&tm);
  json_t *edur = json_object_get(root, "MaxDuration");
   // MaxDuration is a Go time.Duration, marshalled by encoding/json as an
   // int64 count of NANOSECONDS. Convert to seconds for the time_t stop time.
   if( json_is_integer(edur) ) tm_stop = tm_start + json_integer_value(edur)/1000000000LL;
  json_t *acc = json_object_get(root, "TimeAcceleration");
   if( json_is_integer(acc) ) accel= json_integer_value(acc);
    else if( json_is_real(acc) ) accel= json_real_value(acc);

  json_t *flota = json_object_get(root, "FsNodes");
   if( !json_is_array(flota) ) {  std::cerr << "  'FsNodes' is not a JSON table.\n"; return 1; }
   if( (n=json_array_size(flota)) <= 0 ) {  std::cerr << "  no FsNodes in input.\n"; return 1; }
   if( n > MAX_FSNODES ) {  std::cerr << "  " << n << " exceeds sanity limit " << MAX_FSNODES << ".\n"; return 1; }
   // Size tmp[] (used below to stage each satellite's name + TLE lines) to the
   // FsNode count. sat[] is allocated by the caller once r_sat/n_bs are known
   // (a local json_t *sat shadows the global here).
   tmp = (char(*)[3][70]) calloc((size_t)n, sizeof(*tmp));
   if( !tmp ) {  std::cerr << "  cannot allocate tmp for " << n << " FsNodes\n"; return 1; }
  json_array_foreach(flota, index, sat) {
    json_t *tle = json_object_get(sat, "TLE");
    if( tle && !json_is_null(tle) ) { if( parse_sat(sat) ) return 1; }
   }
  json_array_foreach(flota, index, sat) {
    json_t *tle = json_object_get(sat, "TLE");
    if( !tle || json_is_null(tle) ) { if( parse_bs(sat) ) return 1; }
   }

  json_decref(root);
  return 0;
}


static int init_sgp4()
{ int k, tlelinelen=libsgp4::Tle::LineLength();


  for(k=0;k<r_sat;k++)
    if( strlen(tmp[k][1]) == tlelinelen && strlen(tmp[k][2])== tlelinelen ) {
      try {
          libsgp4::Tle tle = libsgp4::Tle(tmp[k][0],tmp[k][1],tmp[k][2]);
          libsgp4::SGP4 sgp4(tle);
          libsgp4::Eci eci = sgp4.FindPosition(sgp4dttm);
          n_sat++;
          V_tle.push_back(tle);
          V_sgp4.push_back(sgp4);
       }
      catch (libsgp4::TleException& e) {
        std::cerr << "  " << tmp[k][0] << ": TLE Error: " << e.what() << std::endl;
       }
      catch (libsgp4::SatelliteException& e) {
        std::cerr << "  " << tmp[k][0] << ": SGP$ Error: " << e.what() << std::endl;
       }
      catch (libsgp4::DecayedException& e) {
        std::cerr << "  " << tmp[k][0] << ": " << e.what() << std::endl;
       }
     }
    else std::cerr << " satellite " << tmp[k][0] << " ignored - wrong TLE lines length\n";

  return 0;
}

static double dist(int s1, int s2)
{ double t, dx, dy, dz, d2, xp, yp, zp;

  dx=sat[s2].x - sat[s1].x; dy=sat[s2].y - sat[s1].y; dz=sat[s2].z - sat[s1].z; d2 = dx*dx + dy*dy + dz*dz;

  // For a satellite<->ground-station link (exactly one endpoint is a base
  // station, index >= n_sat) require the satellite to be at least
  // MIN_GS_ELEVATION_DEG above the ground station's horizon. This subsumes the
  // Earth-occlusion test (elevation >= 0 already means the line clears Earth).
  int gs1 = s1 >= n_sat, gs2 = s2 >= n_sat;
  if( gs1 != gs2 ) {
    int g = gs1 ? s1 : s2, s = gs1 ? s2 : s1;
    double gmag = sqrt(sat[g].x*sat[g].x + sat[g].y*sat[g].y + sat[g].z*sat[g].z);
    double lx=sat[s].x-sat[g].x, ly=sat[s].y-sat[g].y, lz=sat[s].z-sat[g].z;
    double lmag = sqrt(lx*lx + ly*ly + lz*lz);
    if( gmag <= 0.0 || lmag <= 0.0 ) return -sqrt(d2);
    // sin(elevation) = (look vector . local up) / |look|, local up = G/|G|.
    double sinElev = (lx*sat[g].x + ly*sat[g].y + lz*sat[g].z) / (lmag*gmag);
    if( sinElev < sin(MIN_GS_ELEVATION_DEG * DEG2RAD) ) return -sqrt(d2);
    return sqrt(d2);
  }

  t=( -sat[s1].x*dx + -sat[s1].y*dy + -sat[s1].z*dz ) / d2;

  //printf("%2d %2d %f %f\n",s1,s2,t, sqrt(d2));
  if( t<= 0.0 || t >= 1.0 ) return sqrt(d2);
  xp=sat[s1].x+t*dx; yp=sat[s1].y+t*dy; zp=sat[s1].z+t*dz;
  if( sqrt(xp*xp + yp*yp + zp*zp ) <= radiusearthkm ) return -sqrt(d2);
  return sqrt(d2);

}

static int isSunny(double x, double y, double z)
{ double t, dx, dy, dz, d2, xp, yp, zp;

  dx=sun.vx;  dy=sun.vy; dz=sun.vz; d2 = dx*dx + dy*dy + dz*dz;

  t=( -x*dx + -y*dy + -z*dz ) / d2;

  if( t<= 0.0 || t >= 1.0 ) return 1;
  xp=x+t*dx; yp=y+t*dy; zp=z+t*dz;
  if( sqrt(xp*xp + yp*yp + zp*zp ) <= radiusearthkm ) return 0;
  return 1;
}

void subsolarPoint(double JD,double *lat,double *lon)
{
    double n = JD - 2451545.0;
    double L = fmod(280.460 + 0.9856474*n,360.0);
    double g = fmod(357.528 + 0.9856003*n,360.0) * DEG2RAD;
    double lambda = (L + 1.915*sin(g) + 0.020*sin(2*g)) * DEG2RAD;
    double epsilon = (23.439 - 0.0000004*n) * DEG2RAD;
    double alpha = atan2(cos(epsilon)*sin(lambda),cos(lambda));
    double delta = asin(sin(epsilon)*sin(lambda));
    double alpha_deg = alpha * RAD2DEG;
    double delta_deg = delta * RAD2DEG;
    double theta = fmod(280.46061837 +  360.98564736629*(JD-2451545.0),360.0);
    *lon = alpha_deg - theta;
    while(*lon > 180) *lon -= 360;
    while(*lon < -180) *lon += 360;
    *lat = delta_deg;
}

char *  dttm_str(struct tm *info)
{ static char bf[128];
  sprintf(bf,"%4d-%02d-%02d %02d:%02d:%02d",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
  return bf;
}



int calc_pos_proto(int prnt)
{ libsgp4::Vector pos;
  struct tm *info;
  double  d;
  int k,l, ns, nref, bufsize;
  char name[66];
  std::string serialized;


  message.Clear();
  message.set_nsat(n_sat);
  message.set_nbs(n_bs);
  auto* timestamp = message.mutable_time();
  timestamp->set_seconds(tm_sym);
  timestamp->set_nanos(0);

  info = gmtime( &tm_sym );
  sgp4dttm = libsgp4::DateTime(info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);

  subsolarPoint(sgp4dttm.ToJulian(),&sun.lat, &sun.lon);
  libsgp4::Eci bs = libsgp4::Eci(sgp4dttm, sun.lat, sun.lon, 300000.0);
  pos = bs.Position();  sun.vx=pos.x; sun.vy=pos.y; sun.vz=pos.z;
  if(prnt) printf("\nshared memory:\nnsat: %3d\nnbs:  %3d\nutc_dttm: %s,    sun:  lat=%8.2f, lon=%8.2f,   vx=%10.2f, vy=%10.2f, vz=%10.2f\n",
                   n_sat,n_bs,dttm_str(info),sun.lat,sun.lon,sun.vx,sun.vy,sun.vz);


  for(k=ns=0;k<n_sat;k++) {
    libsgp4::Eci eci =  V_sgp4[k].FindPosition(sgp4dttm); pos=eci.Position(); sat[k].x=pos.x; sat[k].y=pos.y; sat[k].z=pos.z;
    libsgp4::CoordGeodetic geo = eci.ToGeodetic();
    strcpy(name,V_tle[k].Name().c_str());

    auto* item = message.add_items();
    item->set_id(k+1);
    item->set_name( name );
    item->set_x(sat[k].x=pos.x);
    item->set_y(sat[k].y=pos.y);
    item->set_z(sat[k].z=pos.z);
    item->set_lat(RAD2DEG * geo.latitude);
    item->set_lon(RAD2DEG * geo.longitude);
    item->set_alt(geo.altitude);
    item->set_in_the_sun(l=isSunny(pos.x,pos.y,pos.z));
    if(l) ns++;
    if(prnt) printf("id:%3d, name=%-24.22s x=%10.2f, y=%10.2f, z=%10.2f, lat=%8.2f, lon=%8.2f, alt=%10.2f, InTheSun=%d\n",
                      k+1,name,pos.x,pos.y,pos.z,RAD2DEG * geo.latitude,RAD2DEG * geo.longitude,geo.altitude,l);
   }

  for(k=0;k<n_bs;k++) {
    strcpy(name, V_bs[k].name);
    libsgp4::Eci bs = libsgp4::Eci(sgp4dttm,V_bs[k].lat, V_bs[k].lon, V_bs[k].alt);
    pos = bs.Position();

    auto* item = message.add_items();
    item->set_id(n_sat+k+1);
    item->set_name( name );
    item->set_x(sat[n_sat+k].x=pos.x);
    item->set_y(sat[n_sat+k].y=pos.y);
    item->set_z(sat[n_sat+k].z=pos.z);
    item->set_lat(V_bs[k].lat);
    item->set_lon(V_bs[k].lon);
    item->set_alt(V_bs[k].alt);
    item->set_in_the_sun(l=isSunny(pos.x,pos.y,pos.z));
    if(prnt) printf("id:%3d, name=%-24.22s x=%10.2f, y=%10.2f, z=%10.2f, lat=%8.2f, lon=%8.2f, alt=%10.2f, InTheSun=%d\n",
                      n_sat+k+1,name,pos.x,pos.y,pos.z,V_bs[k].lat,V_bs[k].lon,V_bs[k].alt,l);

   }


  for(k=nref=0;k<n_sat+n_bs;k++)
    for(l=k+1;l<n_sat+n_bs;l++)
      if(  (d=dist(k,l)) > 0.0 ) {
        // Emit two directed records (a->b and b->a). Previously a single
        // record was added and its id fields were overwritten, so only b->a
        // survived. (The consumer convert.go dedups, so this is safe.)
        auto* distAB = message.add_distances();
        distAB->set_item_id_a(k+1);
        distAB->set_item_id_b(l+1);
        distAB->set_distance(d);
        distAB->set_los(true);
        auto* distBA = message.add_distances();
        distBA->set_item_id_a(l+1);
        distBA->set_item_id_b(k+1);
        distBA->set_distance(d);
        distBA->set_los(true);
        nref+=2;
        if(prnt) printf("item_id_a: %3d, item_id_b: %3d, distance: %8.2f, los: %d\nitem_id_a: %3d, item_id_b: %3d, distance: %8.2f, los: %d\n",
                         k+1,l+1,d,1,l+1,k+1,d,1);
       }
      else {
        auto* distAB = message.add_distances();
        distAB->set_item_id_a(k+1);
        distAB->set_item_id_b(l+1);
        distAB->set_distance(-d);
        distAB->set_los(false);
        auto* distBA = message.add_distances();
        distBA->set_item_id_a(l+1);
        distBA->set_item_id_b(k+1);
        distBA->set_distance(-d);
        distBA->set_los(false);
        if(prnt) printf("item_id_a: %3d, item_id_b: %3d, distance: %8.2f, los: %d\nitem_id_a: %3d, item_id_b: %3d, distance: %8.2f, los: %d\n",
                         k+1,l+1,-d,0,l+1,k+1,-d,0);
       }

  if (!message.SerializeToString(&serialized)) {  perror("SerializeToString failed\n"); return 1; }
  bufsize=serialized.size();

  if(prnt) {
  	printf("\nNumber of sunny satellites: %d\navg nref: %4.1f\n",ns,(float)nref/(n_sat+n_bs));
    printf("\nMessage contains: %d items, %d distances\nRequired buffer size : %d + 5\n",
               message.items_size(),message.distances_size(),bufsize);
    return 0;
   }

  if( !shm_flag ) {
    shm_bufsize=1024 * (int)( 1.1*(bufsize+5)/1024 +1);
    shm = shm_open(SHM_NAME, O_CREAT | O_RDWR, 0666);
    if( shm == -1 ) {  perror("shm_open failed\n"); return 1; }
    ftruncate(shm, shm_bufsize);
    shm_flag=1;
    shm_buf=(char*) mmap(0, shm_bufsize, PROT_WRITE, MAP_SHARED, shm, 0);
    if(shm_buf == MAP_FAILED) {   perror("mmap failed\n");   return 1;  }
    memset(shm_buf,0,shm_bufsize);
   }
  if( bufsize > shm_bufsize ) { perror("buffer size too big\n"); return 1; }
  	
  *shm_buf=0; *(int*)(shm_buf+1)=bufsize; memcpy(shm_buf + 5, serialized.data(), bufsize);
  *shm_buf=0xff;
  return 0;
}

static void int_hnd(int s)
{
cont_flag=0;
}


int main(int argc, char **argv)
{
  libsgp4::Vector pos;
  time_t t0,t1,t2;
  std::tm tm = {};
  int k,l;


  if( argc < 2 || argc > 3 ) { std::cerr << "geo_calc <file name> [<UTC timestamp>]\n"; exit(8); }
  // tmp[] and sat[] are allocated in parse_json once the FsNode count is known.
  if( argc == 3 ) {
    std::istringstream ss(argv[2]);
    ss >> std::get_time(&tm, "%Y-%m-%dT%H:%M:%S");
    if (ss.fail())  { std::cerr << "wrong parameter: " << argv[2] << "\n"; exit(8); }
    //printf("parametr: %d %d %d\n",tm.tm_mday, tm.tm_mon+1, tm.tm_year+1900);
   }

  std::cout << "Parsing file: " << argv[1] << "\n";
   if( parse_json(argv[1]) ) exit(8);
   std::cout << "  n_sat: " << std::setw(5) << r_sat << "\n  n_bs:  " << std::setw(5) << n_bs << "\n";

  // sat[] holds positions for every parsed node (satellites then base
  // stations); it is indexed up to n_sat+n_bs <= r_sat+n_bs.
  sat = (sat_pos_dscr*) calloc((size_t)(r_sat + n_bs), sizeof(*sat));
  if( !sat ) { std::cerr << "cannot allocate sat array for " << (r_sat+n_bs) << " nodes\n"; exit(8); }

  if( argc == 3 ) {
    tm_start=timegm(&tm);
    sgp4dttm = libsgp4::DateTime(tm.tm_year+1900,tm.tm_mon+1,tm.tm_mday);
   }
  std::cout << "Filling SGP4 structures\n";
   //sgp4dttm = libsgp4::DateTime(2027,10,1);
   if( init_sgp4() ) exit(8);
   std::cout << "  n_sat= " << n_sat << " (number of verified satellites)\n";

  free(tmp);

  if( argc == 3 ) {
    tm_sym = tm_start; calc_pos_proto(1);
    shm_unlink(SHM_NAME);
    return 0;
   }

  signal(7,int_hnd);
  signal(2,int_hnd);

  time(&t0); t1=t0;
  while(1) {
    if( !cont_flag ) { printf("signal received\n"); break; }
    usleep(200000); // 200 ms
    time(&t2);
    if(t1 != t2 ) {
      t1 = t2;
      tm_sym = tm_start + (time_t)(accel*(t2-t0));
      if( tm_stop > tm_start && tm_sym > tm_stop ) { printf("end of experiment\n\n"); break; }
      if( calc_pos_proto(0) ) break;
     }
   }

  if(shm_flag ) shm_unlink(SHM_NAME);
  return 0;
}

//g++ -o geo_calc geo_calc.cc geocalc_message.pb.cc  -L. -lsgp4 -ljansson -lprotobuf -std=c++17
